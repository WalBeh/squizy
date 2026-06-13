// Package cli parses arguments and dispatches the list and run subcommands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"squizy/internal/client"
	"squizy/internal/config"
	"squizy/internal/engine"
	"squizy/internal/metrics"
	"squizy/internal/report"
)

const usage = `squizy — token-throughput tester for OpenAI-compatible endpoints

usage:
  squizy list --base-url URL [--api-key KEY]
  squizy run  --base-url URL --model NAME [flags]

run "squizy <command> -h" for command flags.
`

// Main dispatches a subcommand. Returns an error for the process exit code.
func Main(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command given")
	}
	switch args[0] {
	case "list":
		return runList(ctx, args[1:])
	case "run":
		return runRun(ctx, args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	baseURL := fs.String("base-url", "", "server root including /v1 (required)")
	apiKey := fs.String("api-key", "", "bearer token (or SQUIZY_API_KEY)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" {
		return errors.New("--base-url is required")
	}
	cl := client.New(*baseURL, resolveKey(*apiKey))
	models, err := cl.ListModels(ctx)
	if err != nil {
		return err
	}
	report.PrintModels(os.Stdout, models)
	return nil
}

func runRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cfg := &config.RunConfig{}
	var apiKey string

	fs.StringVar(&cfg.BaseURL, "base-url", "", "server root including /v1 (required)")
	fs.StringVar(&cfg.Model, "model", "", "model id from `squizy list` (required)")
	fs.StringVar(&apiKey, "api-key", "", "bearer token (or SQUIZY_API_KEY)")
	fs.StringVar(&cfg.System, "system", "", "system prompt (e.g. 'detailed thinking on' to toggle reasoning)")
	fs.StringVar(&cfg.Task, "task", "prose", "workload prompt style: prose | reason")
	fs.IntVar(&cfg.InputTokens, "input-tokens", 512, "target prompt input length")
	fs.IntVar(&cfg.OutputTokens, "output-tokens", 256, "target answer length (steers prompt)")
	fs.IntVar(&cfg.MaxTokens, "max-tokens", 0, "hard generation cap (0=auto; raise for reasoning models)")
	fs.IntVar(&cfg.Turns, "turns", 1, "turns per conversation (multi-turn when >1)")
	fs.DurationVar(&cfg.ThinkTime, "think-time", time.Second, "pause between a user's turns")
	fs.Float64Var(&cfg.ThinkJitter, "think-jitter", 0.5, "± fraction randomizing think-time")
	fs.BoolVar(&cfg.UniquePrompts, "unique-prompts", true, "unique prefix per request (defeat prefix cache)")
	fs.IntVar(&cfg.StartUsers, "start-users", 1, "sweep start concurrency")
	fs.IntVar(&cfg.MaxUsers, "max-users", 64, "sweep ceiling")
	fs.BoolVar(&cfg.NoSweep, "no-sweep", false, "test only --start-users (no sweep)")
	fs.DurationVar(&cfg.Duration, "duration", 30*time.Second, "per-level run time")
	fs.IntVar(&cfg.Requests, "requests", 0, "per-level request count (overrides --duration)")
	fs.IntVar(&cfg.Warmup, "warmup", 1, "discarded warmup requests")
	fs.DurationVar(&cfg.Timeout, "timeout", 120*time.Second, "per-request timeout")
	fs.IntVar(&cfg.NetProbes, "net-probes", 5, "transport-latency baseline probes at startup (0=skip)")
	fs.DurationVar(&cfg.TTFTSLO, "ttft-slo", 0, "SLO for time-to-first-token, e.g. 2s (0=off)")
	fs.DurationVar(&cfg.E2ESLO, "e2e-slo", 0, "SLO for end-to-end latency, e.g. 30s (0=off)")
	fs.Float64Var(&cfg.KneeGain, "knee-gain", 0.10, "min aggregate improvement to keep ramping")
	fs.Float64Var(&cfg.KneeErrorRate, "knee-error-rate", 0.05, "error rate that stops the sweep")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg.APIKey = resolveKey(apiKey)
	if err := cfg.Validate(); err != nil {
		return err
	}

	cl := client.New(cfg.BaseURL, cfg.APIKey)

	prog := report.NewProgress(os.Stderr, isTerminal(os.Stderr))
	report.PrintRunHeader(os.Stdout, cfg)
	if cfg.NetProbes > 0 {
		report.PrintNetBaseline(os.Stdout, cl.Probe(ctx, cfg.NetProbes))
	}
	report.PrintTableHeader(os.Stdout)

	hooks := engine.Hooks{
		OnWarmup:     func(n int) { prog.Warmup(n) },
		OnLevelStart: func(users int) { prog.LevelStart(users) },
		OnProgress:   func(p engine.LevelProgress) { prog.Update(p) },
		OnLevelDone: func(l metrics.LevelResult) {
			prog.Clear()
			report.PrintLevelRow(os.Stdout, l)
		},
	}

	res := engine.Run(ctx, cl, cfg, hooks)
	prog.Clear()
	report.PrintLatencyDetail(os.Stdout, res.Levels)
	report.PrintSummary(os.Stdout, res, cfg)
	return nil
}

func resolveKey(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return strings.TrimSpace(os.Getenv("SQUIZY_API_KEY"))
}
