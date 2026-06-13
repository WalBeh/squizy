package metrics

import "time"

// LevelResult aggregates all samples from one concurrency level.
type LevelResult struct {
	Users int

	Total    int // requests attempted
	Failed   int
	TimedOut int

	// Throughput (tokens/sec). Generation = think+answer (primary, true hardware
	// rate); answer = useful output only.
	AggregateGen    float64 // system-wide (think+answer) tok/s over the window
	AggregateAnswer float64 // system-wide answer-only tok/s over the window
	PerUserGen      float64 // median per-request whole-generation tok/s
	PerUserAnswer   float64 // median per-request answer-only decode tok/s

	// Latency distributions (seconds, except rates).
	TTFT       Dist
	TTFA       Dist
	E2E        Dist // end-to-end: send -> last token (s)
	GenRate    Dist // per-request whole-generation tok/s
	DecodeRate Dist // per-request answer-only decode tok/s
	InterToken Dist // mean inter-token gap (s)

	// Reasoning.
	AnyThinking    bool
	ThinkLatency   Dist    // first-think -> first-answer (s)
	ThinkTokenFrac float64 // think tokens / all generated tokens

	Estimated  bool // any sample relied on local token estimate
	ShortCount int  // answers below output target

	// SLO attainment: fraction of successful requests meeting the objective.
	// -1 means the objective was not configured.
	TTFTSLOMet float64
	E2ESLOMet  float64

	WindowSeconds float64
}

// ErrorRate is failed (incl. timeouts) over attempted.
func (r LevelResult) ErrorRate() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Failed) / float64(r.Total)
}

// AggregateLevel folds samples for one level into a LevelResult. System
// throughput uses the wall-clock span from the first request sent to the last
// token received across all successful requests — the load the server actually
// sustained. ttftSLO/e2eSLO (0 = off) drive SLO attainment.
func AggregateLevel(users int, samples []RequestSample, ttftSLO, e2eSLO time.Duration) LevelResult {
	r := LevelResult{Users: users, Total: len(samples), TTFTSLOMet: -1, E2ESLOMet: -1}

	var (
		ttft, ttfa, e2e, inter, think []time.Duration
		genRates, decodeRates         []float64
		sumAnswerTok                  int
		sumThinkTok                   int
		minStart, maxEnd              time.Time
		ttftOK, ttftN, e2eOK, e2eN    int
	)

	for _, s := range samples {
		if s.Failed {
			r.Failed++
			if s.TimedOut {
				r.TimedOut++
			}
			continue
		}
		if s.Estimated {
			r.Estimated = true
		}
		if s.Short {
			r.ShortCount++
		}
		if s.HadThinking {
			r.AnyThinking = true
			think = append(think, s.ThinkLatency)
		}
		if s.TTFT > 0 {
			ttft = append(ttft, s.TTFT)
		}
		if s.TTFA > 0 {
			ttfa = append(ttfa, s.TTFA)
		}
		if s.TTFT > 0 && ttftSLO > 0 {
			ttftN++
			if s.TTFT <= ttftSLO {
				ttftOK++
			}
		}
		if !s.Start.IsZero() && s.End.After(s.Start) {
			d := s.End.Sub(s.Start)
			e2e = append(e2e, d)
			if e2eSLO > 0 {
				e2eN++
				if d <= e2eSLO {
					e2eOK++
				}
			}
		}
		if s.InterTokenAvg > 0 {
			inter = append(inter, s.InterTokenAvg)
		}
		if gr := s.GenRate(); gr > 0 {
			genRates = append(genRates, gr)
		}
		if dr := s.DecodeRate(); dr > 0 && !s.Short {
			decodeRates = append(decodeRates, dr)
		}
		sumAnswerTok += s.AnswerTokens
		sumThinkTok += s.ThinkTokens

		if !s.Start.IsZero() && (minStart.IsZero() || s.Start.Before(minStart)) {
			minStart = s.Start
		}
		if s.End.After(maxEnd) {
			maxEnd = s.End
		}
	}

	r.TTFT = durDist(ttft)
	r.TTFA = durDist(ttfa)
	r.E2E = durDist(e2e)
	r.InterToken = durDist(inter)
	r.ThinkLatency = durDist(think)
	r.GenRate = summarize(genRates)
	r.DecodeRate = summarize(decodeRates)
	r.PerUserGen = r.GenRate.P50
	r.PerUserAnswer = r.DecodeRate.P50

	// System throughput over the span [first send, last token received].
	if !minStart.IsZero() && maxEnd.After(minStart) {
		win := maxEnd.Sub(minStart).Seconds()
		r.WindowSeconds = win
		if win > 0 {
			r.AggregateGen = float64(sumAnswerTok+sumThinkTok) / win
			r.AggregateAnswer = float64(sumAnswerTok) / win
		}
	}

	if tot := sumAnswerTok + sumThinkTok; tot > 0 {
		r.ThinkTokenFrac = float64(sumThinkTok) / float64(tot)
	}
	if ttftN > 0 {
		r.TTFTSLOMet = float64(ttftOK) / float64(ttftN)
	}
	if e2eN > 0 {
		r.E2ESLOMet = float64(e2eOK) / float64(e2eN)
	}
	return r
}
