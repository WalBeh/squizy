// Package report renders run results as formatted text.
package report

import (
	"fmt"
	"io"
	"strings"

	"squizy/internal/config"
	"squizy/internal/engine"
	"squizy/internal/metrics"
)

// Header line for the per-level table. "gen" = whole-generation throughput
// (think+answer), the true hardware rate; think% shows the reasoning tax.
const tableHeader = "users   req   err   gen tok/s(agg)  gen/user   think%   TTFT p50/p90    TTFA p50/p90"

// PrintRunHeader echoes the target and configuration.
func PrintRunHeader(w io.Writer, cfg *config.RunConfig) {
	fmt.Fprintf(w, "squizy run — %s @ %s\n", cfg.Model, cfg.BaseURL)
	stop := fmt.Sprintf("%s", cfg.Duration)
	if cfg.StopByRequests() {
		stop = fmt.Sprintf("%d req", cfg.Requests)
	}
	fmt.Fprintf(w, "config: input=%d output=%d turns=%d think=%s±%.0f%% unique-prompts=%v\n",
		cfg.InputTokens, cfg.OutputTokens, cfg.Turns, cfg.ThinkTime, cfg.ThinkJitter*100, cfg.UniquePrompts)
	mode := "sweep"
	if cfg.NoSweep {
		mode = "single-level"
	}
	fmt.Fprintf(w, "        per-level=%s warmup=%d timeout=%s  %s start=%d max=%d\n\n",
		stop, cfg.Warmup, cfg.Timeout, mode, cfg.StartUsers, cfg.MaxUsers)
	fmt.Fprintln(w, tableHeader)
}

// PrintLevelRow renders one completed level as a table row.
func PrintLevelRow(w io.Writer, l metrics.LevelResult) {
	fmt.Fprintf(w, "%5d %5d %5d   %11.1f%s  %8.1f   %5.0f%%   %6s /%6s   %6s /%6s\n",
		l.Users, l.Total, l.Failed,
		l.AggregateGen, estFlag(l), l.PerUserGen, l.ThinkTokenFrac*100,
		secs(l.TTFT.P50), secs(l.TTFT.P90),
		secs(l.TTFA.P50), secs(l.TTFA.P90),
	)
}

func estFlag(l metrics.LevelResult) string {
	if l.Estimated {
		return " ~"
	}
	return ""
}

// PrintLatencyDetail prints full latency distributions per level — the metrics
// that matter most under saturation, where the table's headline throughput hides
// the real user experience. Inter-token latency (stream smoothness) and
// end-to-end (send -> last token) are otherwise invisible.
func PrintLatencyDetail(w io.Writer, levels []metrics.LevelResult) {
	if len(levels) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "latency detail —            p50 /     p90 /     p99 /     max")
	for _, l := range levels {
		fmt.Fprintf(w, "  %d users:\n", l.Users)
		latRow(w, "TTFT", l.TTFT, "s")
		latRow(w, "TTFA", l.TTFA, "s")
		latRow(w, "inter-token", l.InterToken, "ms")
		latRow(w, "end-to-end", l.E2E, "s")
	}
}

// latRow renders one latency metric's distribution. unit "ms" rescales from the
// stored seconds.
func latRow(w io.Writer, name string, d metrics.Dist, unit string) {
	if d.N == 0 {
		fmt.Fprintf(w, "    %-12s        (no samples)\n", name)
		return
	}
	scale := 1.0
	if unit == "ms" {
		scale = 1000.0
	}
	fmt.Fprintf(w, "    %-12s %7.2f / %7.2f / %7.2f / %7.2f %-2s  (n=%d)\n",
		name, d.P50*scale, d.P90*scale, d.P99*scale, d.Max*scale, unit, d.N)
}

// PrintSummary prints the knee verdict, reasoning breakdown, and footnotes.
func PrintSummary(w io.Writer, res engine.SweepResult, cfg *config.RunConfig) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "sweep stopped: %s\n", res.StopReason)

	knee := findLevel(res.Levels, res.KneeUsers)
	if knee != nil {
		fmt.Fprintf(w, "knee: ~%d users — peak aggregate ≈ %.0f gen tok/s; per-user %.0f tok/s (vs %.0f at %d users).\n",
			knee.Users, peakAggregate(res.Levels), knee.PerUserGen, firstPerUser(res.Levels), firstUsers(res.Levels))
	}

	if anyThinking(res.Levels) {
		k := knee
		if k == nil {
			k = lastLevel(res.Levels)
		}
		fmt.Fprintf(w, "reasoning: thinking = %.0f%% of generated tokens; first answer token after %.1fs (p50 thinking wait) at %d users.\n",
			k.ThinkTokenFrac*100, k.ThinkLatency.P50, k.Users)
		fmt.Fprintf(w, "answer-only: useful output %.0f tok/s aggregate, %.0f tok/s per user (excludes thinking).\n",
			k.AggregateAnswer, k.PerUserAnswer)
	}

	if short, est := footnotes(res.Levels); short > 0 || est {
		var notes []string
		if short > 0 {
			notes = append(notes, fmt.Sprintf("%d requests fell short of the %d-token output target (excluded from decode p50)", short, cfg.OutputTokens))
		}
		if est {
			notes = append(notes, "token counts marked ~ are local estimates (server returned no usage)")
		}
		fmt.Fprintf(w, "notes: %s.\n", strings.Join(notes, "; "))
	}
}

// --- helpers ---

func secs(v float64) string {
	if v <= 0 {
		return "   -  "
	}
	return fmt.Sprintf("%.2fs", v)
}

func findLevel(levels []metrics.LevelResult, users int) *metrics.LevelResult {
	for i := range levels {
		if levels[i].Users == users {
			return &levels[i]
		}
	}
	return nil
}

func lastLevel(levels []metrics.LevelResult) *metrics.LevelResult {
	if len(levels) == 0 {
		return nil
	}
	return &levels[len(levels)-1]
}

func peakAggregate(levels []metrics.LevelResult) float64 {
	var max float64
	for _, l := range levels {
		if l.AggregateGen > max {
			max = l.AggregateGen
		}
	}
	return max
}

func firstPerUser(levels []metrics.LevelResult) float64 {
	if len(levels) == 0 {
		return 0
	}
	return levels[0].PerUserGen
}

func firstUsers(levels []metrics.LevelResult) int {
	if len(levels) == 0 {
		return 0
	}
	return levels[0].Users
}

func anyThinking(levels []metrics.LevelResult) bool {
	for _, l := range levels {
		if l.AnyThinking {
			return true
		}
	}
	return false
}

func footnotes(levels []metrics.LevelResult) (short int, estimated bool) {
	for _, l := range levels {
		short += l.ShortCount
		if l.Estimated {
			estimated = true
		}
	}
	return short, estimated
}
