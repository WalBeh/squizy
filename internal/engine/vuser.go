package engine

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"squizy/internal/client"
	"squizy/internal/config"
	"squizy/internal/metrics"
	"squizy/internal/workload"
)

// stopFunc reports whether a virtual user should stop launching new requests.
type stopFunc func() bool

// runVUser drives one virtual user's closed-loop: build a conversation, send
// each turn, record the sample, pause for think-time, repeat — until stop().
func runVUser(
	ctx context.Context,
	cl *client.Client,
	cfg *config.RunConfig,
	seed int64,
	stop stopFunc,
	reserve func() bool, // returns false when the request budget is exhausted
	out chan<- metrics.RequestSample,
) {
	rnd := rand.New(rand.NewSource(seed))

	for !stop() {
		conv := workload.NewConversation(rnd, cfg.System, cfg.Task, cfg.InputTokens, cfg.OutputTokens, cfg.UniquePrompts)
		for turn := 0; turn < cfg.Turns; turn++ {
			if stop() || !reserve() {
				return
			}
			msgs := conv.NextTurn()
			sample, answer := doRequest(ctx, cl, cfg, msgs)
			select {
			case out <- sample:
			case <-ctx.Done():
				return
			}
			conv.RecordAnswer(answer)
			thinkSleep(ctx, rnd, cfg)
		}
	}
}

// doRequest performs one streaming request under a per-request timeout and
// distills it into a sample.
func doRequest(ctx context.Context, cl *client.Client, cfg *config.RunConfig, msgs []client.Message) (metrics.RequestSample, string) {
	reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	obs := cl.StreamChat(reqCtx, client.ChatParams{
		Model:     cfg.Model,
		Messages:  msgs,
		MaxTokens: cfg.EffectiveMaxTokens(),
	})
	timedOut := errors.Is(obs.Err, context.DeadlineExceeded) || reqCtx.Err() == context.DeadlineExceeded
	return metrics.NewSample(obs, cfg.OutputTokens, timedOut), obs.AnswerText
}

// thinkSleep pauses for ThinkTime ± jitter, abortable via context.
func thinkSleep(ctx context.Context, rnd *rand.Rand, cfg *config.RunConfig) {
	if cfg.ThinkTime <= 0 {
		return
	}
	d := cfg.ThinkTime
	if cfg.ThinkJitter > 0 {
		f := 1 + cfg.ThinkJitter*(2*rnd.Float64()-1) // 1 ± jitter
		d = time.Duration(float64(d) * f)
	}
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
