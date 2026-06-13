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

	var think, answer strings.Builder
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

		if d.ReasoningContent != "" {
			if obs.FirstThinkTime.IsZero() {
				obs.FirstThinkTime = now
			}
			think.WriteString(d.ReasoningContent)
			obs.LastTime = now
		}
		if d.Content != "" {
			if obs.FirstAnswerTime.IsZero() {
				obs.FirstAnswerTime = now
			}
			answer.WriteString(d.Content)
			obs.AnswerArrivals = append(obs.AnswerArrivals, now)
			obs.LastTime = now
		}
	}
	if err := sc.Err(); err != nil {
		obs.Err = err
	}

	obs.ThinkText = think.String()
	obs.AnswerText = answer.String()

	// Fallback: some servers inline <think>...</think> in content instead of
	// using reasoning_content. Split it out so reasoning metrics still work.
	if obs.ThinkText == "" {
		if t, a, ok := splitThinkTags(obs.AnswerText); ok {
			obs.ThinkText, obs.AnswerText = t, a
		}
	}
	return obs
}

// splitThinkTags extracts a leading <think>...</think> block from content.
// Returns ok=false when no such block is present.
func splitThinkTags(s string) (think, answer string, ok bool) {
	const open, close = "<think>", "</think>"
	i := strings.Index(s, open)
	if i < 0 {
		return "", "", false
	}
	j := strings.Index(s, close)
	if j < 0 || j < i {
		return "", "", false
	}
	think = strings.TrimSpace(s[i+len(open) : j])
	answer = strings.TrimSpace(s[:i] + s[j+len(close):])
	return think, answer, true
}
