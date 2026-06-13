// Package workload generates synthetic prompts with controlled input/output
// lengths and provides a lightweight local token estimate used when the server
// does not report usage.
package workload

import "strings"

// charsPerToken is a model-family-agnostic heuristic. Real tokenizers vary, so
// any count derived from this is reported as "estimated".
const charsPerToken = 3.7

// EstimateTokens approximates the token count of a string from its length.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := int(float64(len(s))/charsPerToken + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// tokensToWords converts a target token count to an approximate word count
// (~1.3 tokens per word) for steering output length in the prompt text.
func tokensToWords(tokens int) int {
	w := int(float64(tokens) / 1.3)
	if w < 1 {
		w = 1
	}
	return w
}

// approxWordsForTokens builds filler text of roughly the requested token count
// by drawing from the word bank.
func approxWordsForTokens(r randSource, tokens int) string {
	if tokens <= 0 {
		return ""
	}
	var b strings.Builder
	for EstimateTokens(b.String()) < tokens {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(wordBank[r.Intn(len(wordBank))])
	}
	return b.String()
}
