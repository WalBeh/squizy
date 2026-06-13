package cli

import "os"

// isTerminal reports whether f is a character device (a TTY), so progress
// output uses carriage returns only when a human is watching.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
