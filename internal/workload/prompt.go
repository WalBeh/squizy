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
	inputTokens  int
	outputTokens int
	unique       bool
	messages     []client.Message
	turn         int
}

// NewConversation creates a fresh conversation for a virtual user.
func NewConversation(rnd randSource, inputTokens, outputTokens int, unique bool) *Conversation {
	return &Conversation{
		rnd:          rnd,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
		unique:       unique,
	}
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

// NextTurn advances the conversation and returns the message history to send.
// Turn 1 carries the full input-token payload; later turns are short
// follow-ups, relying on accumulated history for growing prefill.
func (c *Conversation) NextTurn() []client.Message {
	c.turn++
	var content string
	if c.turn == 1 {
		filler := approxWordsForTokens(c.rnd, c.inputTokens)
		content = fmt.Sprintf("%s%s\n\nContext:\n%s", c.nonce(), c.steer(), filler)
	} else {
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
