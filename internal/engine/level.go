package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"squizy/internal/client"
	"squizy/internal/config"
	"squizy/internal/metrics"
)

// LevelProgress is emitted periodically while a level runs, for live display.
type LevelProgress struct {
	Users     int
	Completed int
	Failed    int
}

// runLevel runs a single concurrency level and returns its aggregated result.
// Closed-loop semantics: users stop *launching* new requests once the stop
// condition is met, but in-flight requests finish naturally, so timings are
// never truncated.
func runLevel(
	parent context.Context,
	cl *client.Client,
	cfg *config.RunConfig,
	users int,
	onProgress func(LevelProgress),
) metrics.LevelResult {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	samplesCh := make(chan metrics.RequestSample, users*2)

	// Stop condition.
	var (
		budget   int64 = int64(cfg.Requests) // requests mode; <=0 means duration mode
		reserved int64
		deadline time.Time
	)
	if !cfg.StopByRequests() {
		deadline = time.Now().Add(cfg.Duration)
	}
	stop := func() bool {
		if cfg.StopByRequests() {
			return atomic.LoadInt64(&reserved) >= budget
		}
		return time.Now().After(deadline) || ctx.Err() != nil
	}
	reserve := func() bool {
		if !cfg.StopByRequests() {
			return true
		}
		return atomic.AddInt64(&reserved, 1) <= budget
	}

	var wg sync.WaitGroup
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Distinct, deterministic seed per user per level.
			seed := int64(users)*1_000_003 + int64(idx)*7 + 1
			runVUser(ctx, cl, cfg, seed, stop, reserve, samplesCh)
		}(i)
	}

	// Collect samples until all users finish.
	done := make(chan struct{})
	var samples []metrics.RequestSample
	go func() {
		var failed int
		for s := range samplesCh {
			samples = append(samples, s)
			if s.Failed {
				failed++
			}
			if onProgress != nil {
				onProgress(LevelProgress{Users: users, Completed: len(samples), Failed: failed})
			}
		}
		close(done)
	}()

	wg.Wait()
	close(samplesCh)
	<-done

	return metrics.AggregateLevel(users, samples, cfg.TTFTSLO, cfg.E2ESLO)
}

// warmup fires a few discarded requests so model-load / cold-start cost stays
// out of the measured numbers.
func warmup(ctx context.Context, cl *client.Client, cfg *config.RunConfig) {
	for i := 0; i < cfg.Warmup; i++ {
		if ctx.Err() != nil {
			return
		}
		conv := newWarmupConversation(cfg)
		_, _ = doRequest(ctx, cl, cfg, conv)
	}
}

// newWarmupConversation builds a minimal single-turn message for warmup.
func newWarmupConversation(cfg *config.RunConfig) []client.Message {
	return []client.Message{{Role: "user", Content: "Reply with a short sentence to warm up."}}
}
