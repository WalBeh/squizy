// Package config holds the run configuration parsed from CLI flags.
package config

import (
	"errors"
	"time"
)

// RunConfig fully describes one `squizy run` invocation. It flows read-only
// through the engine, metrics, and report layers.
type RunConfig struct {
	BaseURL string
	Model   string
	APIKey  string

	InputTokens   int
	OutputTokens  int
	MaxTokens     int // hard generation cap; 0 = auto from OutputTokens
	Turns         int
	ThinkTime     time.Duration
	ThinkJitter   float64 // ± fraction of ThinkTime
	UniquePrompts bool

	StartUsers int
	MaxUsers   int
	NoSweep    bool

	Duration time.Duration // per-level run time; ignored when Requests > 0
	Requests int           // per-level request count; overrides Duration when > 0

	Warmup    int
	Timeout   time.Duration
	NetProbes int // transport-latency baseline probes at startup (0 = skip)

	TTFTSLO time.Duration // service-level objective for time-to-first-token (0 = off)
	E2ESLO  time.Duration // service-level objective for end-to-end latency (0 = off)

	KneeGain      float64 // min aggregate tok/s improvement to keep ramping
	KneeErrorRate float64 // error rate that halts the sweep
}

// Validate checks required fields and sane ranges.
func (c *RunConfig) Validate() error {
	if c.BaseURL == "" {
		return errors.New("--base-url is required")
	}
	if c.Model == "" {
		return errors.New("--model is required (use `squizy list` to see options)")
	}
	if c.InputTokens < 1 {
		return errors.New("--input-tokens must be >= 1")
	}
	if c.OutputTokens < 1 {
		return errors.New("--output-tokens must be >= 1")
	}
	if c.Turns < 1 {
		return errors.New("--turns must be >= 1")
	}
	if c.StartUsers < 1 {
		return errors.New("--start-users must be >= 1")
	}
	if !c.NoSweep && c.MaxUsers < c.StartUsers {
		return errors.New("--max-users must be >= --start-users")
	}
	if c.Requests == 0 && c.Duration <= 0 {
		return errors.New("set --duration or --requests")
	}
	return nil
}

// StopByRequests reports whether levels stop after a fixed request count.
func (c *RunConfig) StopByRequests() bool { return c.Requests > 0 }

// HasSLO reports whether any service-level objective is configured.
func (c *RunConfig) HasSLO() bool { return c.TTFTSLO > 0 || c.E2ESLO > 0 }

// EffectiveMaxTokens is the hard generation cap sent to the server. When
// MaxTokens is unset it leaves headroom above the output target; for reasoning
// models, set MaxTokens explicitly so thinking does not starve the answer.
func (c *RunConfig) EffectiveMaxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return c.OutputTokens + c.OutputTokens/2 + 64
}
