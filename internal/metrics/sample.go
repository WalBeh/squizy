// Package metrics turns raw stream observations into per-request samples and
// aggregates them into per-level statistics.
package metrics

import (
	"time"

	"squizy/internal/client"
	"squizy/internal/workload"
)

// RequestSample is the distilled, per-request measurement.
type RequestSample struct {
	Failed   bool
	TimedOut bool

	// Latency (client-side).
	TTFT time.Duration // send -> first token (think or answer)
	TTFA time.Duration // send -> first answer token (0 if no answer ever arrived)

	HadThinking   bool
	ThinkLatency  time.Duration // first-think -> first-answer (the thinking wait)
	GenWindow     time.Duration // first token -> last token (whole generation)
	DecodeWindow  time.Duration // first-answer -> last answer token
	InterTokenAvg time.Duration // mean gap between answer deltas

	AnswerTokens int
	ThinkTokens  int
	Estimated    bool // true when counts came from the local heuristic (no server usage)
	Short        bool // answer fell short of target output length

	Start time.Time // request send time (for windowing)
	End   time.Time // last token time (for windowing)
}

// GenRate is the per-request whole-generation speed (think+answer) in
// tokens/sec — the true rate the hardware produces tokens, the primary
// throughput number. For non-reasoning models it equals the answer rate.
func (s RequestSample) GenRate() float64 {
	tot := s.ThinkTokens + s.AnswerTokens
	if s.GenWindow <= 0 || tot <= 0 {
		return 0
	}
	return float64(tot) / s.GenWindow.Seconds()
}

// DecodeRate is the per-request answer-only decode speed in tokens/sec — the
// "useful" rate a user perceives once thinking is done.
func (s RequestSample) DecodeRate() float64 {
	if s.DecodeWindow <= 0 || s.AnswerTokens <= 0 {
		return 0
	}
	return float64(s.AnswerTokens) / s.DecodeWindow.Seconds()
}

// NewSample builds a sample from a stream observation and the target output
// length (to flag short answers). timedOut marks context-deadline failures.
func NewSample(obs *client.StreamObservation, targetOutput int, timedOut bool) RequestSample {
	s := RequestSample{
		Start:    obs.SendTime,
		End:      obs.LastTime,
		TimedOut: timedOut,
	}
	if obs.Err != nil {
		s.Failed = true
	}

	firstToken := earliest(obs.FirstThinkTime, obs.FirstAnswerTime)
	if !firstToken.IsZero() {
		s.TTFT = firstToken.Sub(obs.SendTime)
		if !obs.LastTime.IsZero() {
			s.GenWindow = obs.LastTime.Sub(firstToken)
		}
	}
	if !obs.FirstAnswerTime.IsZero() {
		s.TTFA = obs.FirstAnswerTime.Sub(obs.SendTime)
		if !obs.LastTime.IsZero() {
			s.DecodeWindow = obs.LastTime.Sub(obs.FirstAnswerTime)
		}
	}
	if obs.HasThinking() {
		s.HadThinking = true
		if !obs.FirstAnswerTime.IsZero() {
			s.ThinkLatency = obs.FirstAnswerTime.Sub(obs.FirstThinkTime)
		}
	}
	s.InterTokenAvg = meanGap(obs.AnswerArrivals)

	// Token counts: prefer the server's total, split think/answer by text ratio;
	// fall back to a flagged local estimate.
	localThink := workload.EstimateTokens(obs.ThinkText)
	localAnswer := workload.EstimateTokens(obs.AnswerText)
	if obs.Usage != nil && obs.Usage.CompletionTokens > 0 {
		total := obs.Usage.CompletionTokens
		if sum := localThink + localAnswer; sum > 0 {
			s.ThinkTokens = total * localThink / sum
			s.AnswerTokens = total - s.ThinkTokens
		} else {
			s.AnswerTokens = total
		}
	} else {
		s.ThinkTokens = localThink
		s.AnswerTokens = localAnswer
		s.Estimated = true
	}

	if s.AnswerTokens < (targetOutput*7)/10 {
		s.Short = true
	}
	return s
}

func earliest(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case a.Before(b):
		return a
	default:
		return b
	}
}

func meanGap(ts []time.Time) time.Duration {
	if len(ts) < 2 {
		return 0
	}
	total := ts[len(ts)-1].Sub(ts[0])
	return total / time.Duration(len(ts)-1)
}
