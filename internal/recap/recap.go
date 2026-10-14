// Package recap writes a Recap: three short lines (goal, done, next) a small model
// writes about a Session, kept in unpause's cache and never in the Transcript (ADR 0012).
package recap

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hamzafer/unpause/internal/session"
)

// MaxInput bounds what the model sees, in characters.
const MaxInput = 8000

// maxFirst and maxMessage cap the first prompt and every other message, so one pasted log
// can't crowd out the rest of the talk.
const (
	maxFirst   = 2000
	maxMessage = 1500
)

// Input is what the model sees: repo, branch and Title, then the Session's talk in file
// order. When the talk doesn't fit in MaxInput, the first prompt and as much of the end
// as fits are kept and the middle is left out.
func Input(s *session.Session, talk []session.Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repo: %s\n", s.Repo())
	if s.Branch != "" {
		fmt.Fprintf(&b, "Branch: %s\n", s.Branch)
	}
	fmt.Fprintf(&b, "Title: %s\n\nConversation:\n", session.Clip(s.Title(), 200))
	header := b.String()

	first := -1
	for i, m := range talk {
		if m.Role == "user" {
			first = i
			break
		}
	}
	var head string
	if first >= 0 {
		head = line(talk[first], maxFirst)
	}
	const gapReserve = 48 // room for the "messages left out" line
	budget := MaxInput - utf8.RuneCountInString(header) - utf8.RuneCountInString(head) - gapReserve

	var tail []string
	from := len(talk)
	for i := len(talk) - 1; i > first; i-- {
		l := line(talk[i], maxMessage)
		n := utf8.RuneCountInString(l)
		if n > budget {
			break
		}
		budget -= n
		tail = append(tail, l)
		from = i
	}

	b.WriteString(head)
	if skipped := from - first - 1; skipped > 0 {
		fmt.Fprintf(&b, "[… %d messages left out …]\n", skipped)
	}
	for i := len(tail) - 1; i >= 0; i-- {
		b.WriteString(tail[i])
	}
	return b.String()
}

// Recap is what the model said about a Session, and the transcript size it was made from.
type Recap struct {
	Goal string    `json:"goal"`
	Done string    `json:"done"`
	Next string    `json:"next"`
	Size int64     `json:"size"` // bytes of the transcript when the Recap was made
	At   time.Time `json:"at"`
}

// MaxLine is the longest line Parse accepts. The model is asked for at most 100 characters.
const MaxLine = 120

// Parse reads a model reply into a Recap. It needs exactly one line each labelled goal,
// done and next (any case, an optional colon, list markers and bold allowed); other
// lines are ignored. Anything else is an error, so a bad reply is never stored.
func Parse(reply string) (Recap, error) {
	var r Recap
	fields := map[string]*string{"goal": &r.Goal, "done": &r.Done, "next": &r.Next}
	for _, raw := range strings.Split(reply, "\n") {
		l := strings.ReplaceAll(strings.ReplaceAll(raw, "**", ""), "__", "")
		l = strings.TrimLeft(strings.TrimSpace(l), "-*•> \t")
		if len(l) < 4 {
			continue
		}
		label := strings.ToLower(l[:4])
		dst, ok := fields[label]
		rest := l[4:]
		if !ok || (rest != "" && rest[0] != ':' && rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		if *dst != "" {
			return Recap{}, fmt.Errorf("recap reply has more than one %s line", label)
		}
		v := strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		if v == "" {
			return Recap{}, fmt.Errorf("recap reply has an empty %s line", label)
		}
		if utf8.RuneCountInString(v) > MaxLine {
			return Recap{}, fmt.Errorf("recap reply's %s line is longer than %d characters", label, MaxLine)
		}
		*dst = v
	}
	for _, label := range []string{"goal", "done", "next"} {
		if *fields[label] == "" {
			return Recap{}, fmt.Errorf("recap reply has no %s line", label)
		}
	}
	return r, nil
}

// line formats one message for the input, cut to max characters.
func line(m session.Message, max int) string {
	who := "Claude"
	if m.Role == "user" {
		who = "User"
	}
	return who + ": " + clipKeepingNewlines(strings.TrimSpace(m.Text), max) + "\n"
}

// clipKeepingNewlines cuts s to at most max characters, ending in "…" when it was cut.
// Unlike session.Clip, which collapses whitespace for one-line display, it leaves line
// breaks alone, so a message keeps its shape in the model's input.
func clipKeepingNewlines(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	i, n := 0, 0
	for n < max-1 {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return s[:i] + "…"
}
