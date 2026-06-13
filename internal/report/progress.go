package report

import (
	"fmt"
	"io"

	"squizy/internal/engine"
)

// Progress renders a transient, single-line status to a (typically stderr)
// writer using carriage returns. It is silent when disabled (e.g. non-TTY).
type Progress struct {
	w       io.Writer
	enabled bool
	lastLen int
}

// NewProgress builds a progress printer. Pass enabled=false to disable (no TTY).
func NewProgress(w io.Writer, enabled bool) *Progress {
	return &Progress{w: w, enabled: enabled}
}

// LevelStart announces a new concurrency level.
func (p *Progress) LevelStart(users int) {
	if !p.enabled {
		return
	}
	p.line(fmt.Sprintf("level %d users: starting…", users))
}

// Update refreshes the in-place counter for the running level.
func (p *Progress) Update(pr engine.LevelProgress) {
	if !p.enabled {
		return
	}
	p.line(fmt.Sprintf("level %d users: %d done, %d failed…", pr.Users, pr.Completed, pr.Failed))
}

// Warmup announces warmup requests.
func (p *Progress) Warmup(n int) {
	if !p.enabled {
		return
	}
	p.line(fmt.Sprintf("warmup: %d request(s)…", n))
}

// Clear erases the transient line so the final report prints cleanly.
func (p *Progress) Clear() {
	if !p.enabled || p.lastLen == 0 {
		return
	}
	fmt.Fprintf(p.w, "\r%*s\r", p.lastLen, "")
	p.lastLen = 0
}

func (p *Progress) line(s string) {
	// Pad to overwrite any previous, longer line.
	pad := ""
	if len(s) < p.lastLen {
		pad = fmt.Sprintf("%*s", p.lastLen-len(s), "")
	}
	fmt.Fprintf(p.w, "\r%s%s", s, pad)
	p.lastLen = len(s)
}
