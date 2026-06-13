package client

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"time"
)

// StreamObservation is the raw, client-side-timed result of one streaming
// request. The metrics layer turns these into statistics.
type StreamObservation struct {
	SendTime        time.Time   // request written to the wire
	FirstThinkTime  time.Time   // first non-empty reasoning_content delta (zero if none)
	FirstAnswerTime time.Time   // first non-empty content delta (zero if none)
	LastTime        time.Time   // arrival of the final content-bearing delta
	AnswerArrivals  []time.Time // arrival time of each non-empty answer delta (for inter-token latency)

	ThinkText  string // accumulated reasoning_content
	AnswerText string // accumulated content

	Usage        *Usage // server-reported token usage, nil if absent
	FinishReason string
	Err          error // non-nil if the request failed; partial fields may still be set
}

// HasThinking reports whether any reasoning tokens were observed.
func (o *StreamObservation) HasThinking() bool { return !o.FirstThinkTime.IsZero() }

// StreamChat issues one streaming chat request and returns a fully-timed
// observation. A non-nil error is also stored in obs.Err so callers can record
// the partial sample. Keepalive chunks and role-only/empty deltas are skipped:
// the first *real* token is the first non-empty content or reasoning_content
// delta from a non-keepalive chunk.
func (c *Client) StreamChat(ctx context.Context, p ChatParams) *StreamObservation {
	obs := &StreamObservation{SendTime: time.Now()}

	resp, err := c.openStream(ctx, p)
	if err != nil {
		obs.Err = err
		return obs
	}
	defer resp.Body.Close()

	var think, content strings.Builder
	var firstContentTime, crossedTime time.Time
	var allArrivals []time.Time
	var tagTail string // sliding suffix to catch </think> split across deltas

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var ch chunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			// Tolerate a malformed chunk rather than failing the whole request.
			continue
		}
		// omlx (and others) send a placeholder keepalive chunk during prefill,
		// before any real token. It must not start the TTFT clock.
		if ch.Model == "keepalive" {
			continue
		}
		if ch.Usage != nil {
			obs.Usage = ch.Usage
		}
		if len(ch.Choices) == 0 {
			continue
		}
		now := time.Now()
		d := ch.Choices[0].Delta
		if fr := ch.Choices[0].FinishReason; fr != nil && *fr != "" {
			obs.FinishReason = *fr
		}

		// Path 1: server exposes a dedicated reasoning field (omlx, vLLM with a
		// reasoning parser). Thinking and answer arrive pre-separated.
		if rc := d.reasoning(); rc != "" {
			if obs.FirstThinkTime.IsZero() {
				obs.FirstThinkTime = now
			}
			think.WriteString(rc)
			obs.LastTime = now
		}
		// Path 2: content. May be a pure answer, or inline reasoning ending in
		// </think> (Nemotron/DeepSeek with no parser — the template pre-opens the
		// tag, so only the closing tag appears). Detect the boundary live so TTFA
		// reflects the first real answer token, not the first thinking token.
		if d.Content != "" {
			if firstContentTime.IsZero() {
				firstContentTime = now
			}
			content.WriteString(d.Content)
			allArrivals = append(allArrivals, now)
			obs.LastTime = now
			if crossedTime.IsZero() {
				if strings.Contains(tagTail+d.Content, "</think>") {
					crossedTime = now
				} else {
					tagTail = lastN(tagTail+d.Content, 7)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		obs.Err = err
	}

	obs.ThinkText = think.String()
	full := content.String()

	switch {
	case obs.ThinkText != "":
		// Dedicated reasoning field used; all content is the answer.
		obs.AnswerText = full
		if obs.FirstAnswerTime.IsZero() {
			obs.FirstAnswerTime = firstContentTime
		}
		obs.AnswerArrivals = allArrivals
	case !crossedTime.IsZero():
		// Inline reasoning: split at </think>; thinking started with the first
		// content token, the answer at the boundary.
		obs.ThinkText, obs.AnswerText = splitInline(full)
		obs.FirstThinkTime = firstContentTime
		obs.FirstAnswerTime = crossedTime
		obs.AnswerArrivals = after(allArrivals, crossedTime)
	default:
		// No reasoning: content is the whole answer.
		obs.AnswerText = full
		obs.FirstAnswerTime = firstContentTime
		obs.AnswerArrivals = allArrivals
	}
	return obs
}

// splitInline splits content at the first </think>, stripping a leading <think>
// if the model emitted one. Caller guarantees </think> is present.
func splitInline(s string) (think, answer string) {
	const open, close = "<think>", "</think>"
	j := strings.Index(s, close)
	if j < 0 {
		return "", strings.TrimSpace(s)
	}
	start := 0
	if i := strings.Index(s[:j], open); i >= 0 {
		start = i + len(open)
	}
	return strings.TrimSpace(s[start:j]), strings.TrimSpace(s[j+len(close):])
}

// after returns the timestamps strictly later than t.
func after(ts []time.Time, t time.Time) []time.Time {
	var out []time.Time
	for _, x := range ts {
		if x.After(t) {
			out = append(out, x)
		}
	}
	return out
}

// lastN returns the last n bytes of s (all of s if shorter).
func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
