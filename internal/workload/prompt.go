package workload

import (
	"fmt"

	"squizy/internal/client"
)

// randSource is the subset of *math/rand.Rand we use; lets the engine pass a
// per-virtual-user seeded source for reproducibility.
type randSource interface {
	Intn(n int) int
	Int63() int64
}

// Conversation builds the message history for one virtual user across turns.
// Each user owns a Conversation so multi-turn context accumulates naturally:
// every turn resends the full history, so prefill cost climbs.
type Conversation struct {
	rnd          randSource
	system       string
	task         string // "prose" (default) or "reason"
	inputTokens  int
	outputTokens int
	unique       bool
	messages     []client.Message
	turn         int
}

// NewConversation creates a fresh conversation for a virtual user. A non-empty
// system prompt is seeded as the first message (used e.g. to toggle reasoning).
// task selects the prompt style: "prose" (generative) or "reason" (a randomized
// step-by-step problem that triggers a model's reasoning block).
func NewConversation(rnd randSource, system, task string, inputTokens, outputTokens int, unique bool) *Conversation {
	c := &Conversation{
		rnd:          rnd,
		system:       system,
		task:         task,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
		unique:       unique,
	}
	if system != "" {
		c.messages = append(c.messages, client.Message{Role: "system", Content: system})
	}
	return c
}

// nonce returns a per-request prefix. When unique is true it varies every call
// (placed at the very start so prefixes differ and prefix-caches miss); when
// false it is empty so every request shares an identical, cacheable prompt.
func (c *Conversation) nonce() string {
	if !c.unique {
		return ""
	}
	return fmt.Sprintf("[ref:%012x] ", c.rnd.Int63())
}

// steer is the instruction that pushes the model toward the target output
// length. max_tokens only caps; this nudges the model to actually fill it.
func (c *Conversation) steer() string {
	return fmt.Sprintf("Write approximately %d words of detailed, continuous prose. Do not stop early and do not ask questions.", tokensToWords(c.outputTokens))
}

// reasonPrompt builds a randomized, self-contained multi-step word problem that
// reliably triggers a reasoning model's thinking block. Numbers vary per request
// so prefixes differ (cache realism). Filler pads toward the input-token target.
func (c *Conversation) reasonPrompt() string {
	a := 100 + c.rnd.Intn(900)
	b := 10 + c.rnd.Intn(90)
	days := 3 + c.rnd.Intn(20)
	ret := c.rnd.Intn(60)
	thresh := 2000 + c.rnd.Intn(20000)
	problem := fmt.Sprintf(
		"A warehouse ships %d units on day 1, then %d units on each of the next %d days, but %d units are returned in total. Reason step by step: compute the net total shipped, the average shipped per day across all days, and state clearly whether the net total exceeds %d.",
		a, b, days, ret, thresh)
	pad := c.inputTokens - EstimateTokens(problem)
	if pad < 0 {
		pad = 0
	}
	filler := approxWordsForTokens(c.rnd, pad)
	return fmt.Sprintf("%s%s\n\nBackground (ignore if irrelevant): %s", c.nonce(), problem, filler)
}

// NextTurn advances the conversation and returns the message history to send.
// Turn 1 carries the full input-token payload; later turns are short
// follow-ups, relying on accumulated history for growing prefill.
func (c *Conversation) NextTurn() []client.Message {
	c.turn++
	var content string
	switch {
	case c.task == "reason" && c.turn == 1:
		content = c.reasonPrompt()
	case c.task == "reason":
		a := 10 + c.rnd.Intn(90)
		content = fmt.Sprintf("%sNow redo the calculation if the daily shipment increases by %d units. Reason step by step.", c.nonce(), a)
	case c.turn == 1:
		filler := approxWordsForTokens(c.rnd, c.inputTokens)
		content = fmt.Sprintf("%s%s\n\nContext:\n%s", c.nonce(), c.steer(), filler)
	default:
		content = fmt.Sprintf("%sContinue with more detail on a different aspect. %s", c.nonce(), c.steer())
	}
	c.messages = append(c.messages, client.Message{Role: "user", Content: content})
	// Return a copy so the caller can't mutate our history.
	out := make([]client.Message, len(c.messages))
	copy(out, c.messages)
	return out
}

// RecordAnswer appends the assistant's reply so the next turn includes it.
func (c *Conversation) RecordAnswer(answer string) {
	c.messages = append(c.messages, client.Message{Role: "assistant", Content: answer})
}
