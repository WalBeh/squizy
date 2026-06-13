package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"squizy/internal/client"
)

// nonChatHints are id substrings that usually indicate a model that is not a
// chat-completion target (embeddings, rerankers, OCR/vision-only, converters).
var nonChatHints = []string{"embed", "modernbert", "rerank", "ocr", "markitdown", "whisper", "tts"}

// PrintModels lists models, flagging those that look like non-chat targets.
func PrintModels(w io.Writer, models []client.Model) {
	if len(models) == 0 {
		fmt.Fprintln(w, "no models reported by endpoint")
		return
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	fmt.Fprintf(w, "%-36s %12s  %s\n", "MODEL", "MAX_LEN", "NOTE")
	for _, m := range models {
		maxLen := "-"
		if m.MaxModelLen != nil {
			maxLen = fmt.Sprintf("%d", *m.MaxModelLen)
		}
		note := ""
		if !chatLikely(m.ID) {
			note = "non-chat? (skip for throughput)"
		}
		fmt.Fprintf(w, "%-36s %12s  %s\n", m.ID, maxLen, note)
	}
}

func chatLikely(id string) bool {
	lower := strings.ToLower(id)
	for _, h := range nonChatHints {
		if strings.Contains(lower, h) {
			return false
		}
	}
	return true
}
