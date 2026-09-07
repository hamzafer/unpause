package session

import (
	"fmt"
	"strings"
	"time"
)

// Age renders a compact relative time: now, 5m, 3h, 2d, 4mo.
func Age(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
}

// Clip collapses newlines and truncates to n runes with an ellipsis.
func Clip(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return string(r)
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
