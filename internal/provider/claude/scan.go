// Package claude reads Claude Code's on-disk session data.
//
// Layout (Claude Code 2.1.x):
//
//	<root>/projects/<encoded-cwd>/<session-id>.jsonl   transcript, one JSON object per line
//	<root>/projects/<encoded-cwd>/<session-id>/...      subagent transcripts (ignored)
//	<root>/sessions/<pid>.json                          live session marker
//
// The transcript format is internal to Claude Code and may change. The parser
// only depends on a handful of fields and ignores everything else.
package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

// PreviewLen is how many trailing messages we keep for the preview pane.
const PreviewLen = 8

// maxPromptLen caps stored prompt text so the cache stays small.
const maxPromptLen = 400

// line is the subset of a transcript record we care about.
type line struct {
	Type        string          `json:"type"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	IsSidechain bool            `json:"isSidechain"`
	SessionID   string          `json:"sessionId"`
	CustomTitle string          `json:"customTitle"`
	AITitle     string          `json:"aiTitle"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ScanFile parses one transcript. It streams the whole file once; callers cache the result by mtime.
func ScanFile(path string) (*session.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := Scan(f)
	if err != nil {
		return nil, err
	}
	s.Path = path
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if st, err := f.Stat(); err == nil && s.LastActive.IsZero() {
		s.LastActive = st.ModTime()
	}
	return s, nil
}

// Scan parses transcript lines from r.
func Scan(r io.Reader) (*session.Session, error) {
	s := &session.Session{Provider: "claude"}
	br := bufio.NewReaderSize(r, 1<<20)
	ring := make([]session.Message, 0, PreviewLen)
	push := func(m session.Message) {
		if len(ring) == PreviewLen {
			copy(ring, ring[1:])
			ring = ring[:PreviewLen-1]
		}
		ring = append(ring, m)
	}

	for {
		raw, err := br.ReadBytes('\n')
		if len(raw) > 0 {
			consume(s, bytes.TrimSpace(raw), push)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	s.Preview = ring
	return s, nil
}

// interesting is a cheap pre-filter so we only JSON-decode lines that can matter.
func interesting(raw []byte) bool {
	return bytes.Contains(raw, []byte(`"type":"user"`)) ||
		bytes.Contains(raw, []byte(`"type":"assistant"`)) ||
		bytes.Contains(raw, []byte(`"type":"custom-title"`)) ||
		bytes.Contains(raw, []byte(`"type":"ai-title"`))
}

func consume(s *session.Session, raw []byte, push func(session.Message)) {
	if len(raw) == 0 || raw[0] != '{' || !interesting(raw) {
		return
	}
	var l line
	if err := json.Unmarshal(raw, &l); err != nil {
		return
	}
	if s.ID == "" && l.SessionID != "" {
		s.ID = l.SessionID
	}
	switch l.Type {
	case "custom-title":
		s.CustomTitle = strings.TrimSpace(l.CustomTitle)
	case "ai-title":
		s.AITitle = strings.TrimSpace(l.AITitle)
	case "user", "assistant":
		if l.CWD != "" {
			s.CWD = l.CWD
		}
		if l.GitBranch != "" {
			s.Branch = l.GitBranch
		}
		if l.IsSidechain {
			s.Sidechain = true
		}
		at, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
		if !at.IsZero() {
			s.LastActive = at // any activity counts, tool results included
		}
		text := extractText(l.Message)
		if text == "" {
			return
		}
		if l.Type == "user" && isCommandNoise(text) {
			return
		}
		if s.Created.IsZero() && !at.IsZero() {
			s.Created = at
		}
		s.Messages++
		if l.Type == "user" && s.FirstPrompt == "" {
			s.FirstPrompt = truncate(text, maxPromptLen)
		}
		push(session.Message{Role: l.Type, Text: truncate(text, maxPromptLen), At: at})
	}
}

// extractText returns the human-readable text of a message, skipping tool calls, tool results and thinking.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m message
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if len(m.Content) == 0 {
		return ""
	}
	if m.Content[0] == '"' {
		var str string
		if json.Unmarshal(m.Content, &str) == nil {
			return strings.TrimSpace(str)
		}
		return ""
	}
	var blocks []block
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n")
}

// isCommandNoise drops the synthetic user lines Claude Code writes for slash commands and hooks.
func isCommandNoise(t string) bool {
	return strings.HasPrefix(t, "<command-name>") ||
		strings.HasPrefix(t, "<command-message>") ||
		strings.HasPrefix(t, "Base directory for this skill:") ||
		strings.HasPrefix(t, "<bash-input>") ||
		strings.HasPrefix(t, "<bash-stdout>") ||
		strings.HasPrefix(t, "<local-command-stdout>") ||
		strings.HasPrefix(t, "<local-command-caveat>") ||
		strings.HasPrefix(t, "<system-reminder>") ||
		strings.HasPrefix(t, "Caveat: The messages below")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
