package engine

import (
	"context"

	"squizy/internal/client"
	"squizy/internal/config"
	"squizy/internal/metrics"
)

// Hooks let the caller observe the sweep live without coupling the engine to a
// particular output format.
type Hooks struct {
	OnWarmup     func(n int)
	OnLevelStart func(users int)
	OnProgress   func(LevelProgress)
	OnLevelDone  func(metrics.LevelResult)
}

// SweepResult is the full outcome of a run.
type SweepResult struct {
	Levels     []metrics.LevelResult
	KneeUsers  int // users at the saturation knee (0 if not identified)
	StopReason string
}

// levelSequence yields the concurrency levels to test: start, doubling, capped
// at MaxUsers (which is always included as the final level).
func levelSequence(cfg *config.RunConfig) []int {
	if cfg.NoSweep {
		return []int{cfg.StartUsers}
	}
	var seq []int
	for u := cfg.StartUsers; u < cfg.MaxUsers; u *= 2 {
		seq = append(seq, u)
	}
	seq = append(seq, cfg.MaxUsers)
	return seq
}

// Run executes warmup and the concurrency sweep, applying knee detection.
func Run(ctx context.Context, cl *client.Client, cfg *config.RunConfig, h Hooks) SweepResult {
	if cfg.Warmup > 0 {
		if h.OnWarmup != nil {
			h.OnWarmup(cfg.Warmup)
		}
		warmup(ctx, cl, cfg)
	}

	var res SweepResult
	for _, users := range levelSequence(cfg) {
		if ctx.Err() != nil {
			res.StopReason = "cancelled"
			break
		}
		if h.OnLevelStart != nil {
			h.OnLevelStart(users)
		}
		lvl := runLevel(ctx, cl, cfg, users, h.OnProgress)
		res.Levels = append(res.Levels, lvl)
		if h.OnLevelDone != nil {
			h.OnLevelDone(lvl)
		}

		if cfg.NoSweep {
			res.StopReason = "single level (--no-sweep)"
			break
		}
		if stop, reason, knee := kneeReached(res.Levels, cfg); stop {
			res.StopReason = reason
			res.KneeUsers = knee
			break
		}
	}
	if res.StopReason == "" {
		res.StopReason = "reached --max-users"
		if n := len(res.Levels); n > 0 {
			res.KneeUsers = res.Levels[n-1].Users
		}
	}
	return res
}

// kneeReached inspects the latest two levels for saturation or excessive errors.
func kneeReached(levels []metrics.LevelResult, cfg *config.RunConfig) (stop bool, reason string, knee int) {
	cur := levels[len(levels)-1]
	if cur.ErrorRate() > cfg.KneeErrorRate {
		return true, "error rate exceeded threshold", kneeOf(levels)
	}
	if len(levels) < 2 {
		return false, "", 0
	}
	prev := levels[len(levels)-2]
	if prev.AggregateGen <= 0 {
		return false, "", 0
	}
	gain := (cur.AggregateGen - prev.AggregateGen) / prev.AggregateGen
	if gain < cfg.KneeGain {
		return true, "aggregate throughput plateaued", prev.Users
	}
	return false, "", 0
}

// kneeOf picks the level with the highest aggregate throughput as the knee.
func kneeOf(levels []metrics.LevelResult) int {
	best, bestUsers := -1.0, 0
	for _, l := range levels {
		if l.AggregateGen > best {
			best, bestUsers = l.AggregateGen, l.Users
		}
	}
	return bestUsers
}
