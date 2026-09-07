// Package session defines the provider-agnostic model of a coding session.
package session

import (
	"path/filepath"
	"time"
)

// Message is one line of conversation kept for the preview pane.
type Message struct {
	Role string    `json:"role"` // "user" or "assistant"
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Live describes a session that is running right now.
type Live struct {
	PID    int    `json:"pid"`
	Status string `json:"status"` // "busy", "idle", ...
	Name   string `json:"name"`
}

// Session is one resumable conversation, regardless of which tool produced it.
type Session struct {
	Provider string `json:"provider"` // "claude"
	ID       string `json:"id"`
	Account  string `json:"account"` // label of the config root, e.g. "personal"
	Root     string `json:"root"`    // config dir the session belongs to
	Path     string `json:"path"`    // transcript on disk

	CWD    string `json:"cwd"`
	Branch string `json:"branch,omitempty"`

	CustomTitle string `json:"custom_title,omitempty"`
	AITitle     string `json:"ai_title,omitempty"`
	FirstPrompt string `json:"first_prompt,omitempty"`

	Created    time.Time `json:"created"`
	LastActive time.Time `json:"last_active"`
	Messages   int       `json:"messages"` // user prompts + assistant replies
	Sidechain  bool      `json:"sidechain"`
	// ContextTokens is the context size reported by the last assistant reply
	// (input + cache creation + cache read). Zero when unknown.
	ContextTokens int `json:"context_tokens,omitempty"`

	Preview []Message `json:"preview,omitempty"`

	// Filled at query time, never cached.
	Live       *Live `json:"live,omitempty"`
	CWDMissing bool  `json:"cwd_missing,omitempty"`
}

// Title is the best human name we have: /rename name, else Claude's auto title, else the first prompt.
func (s *Session) Title() string {
	switch {
	case s.CustomTitle != "":
		return s.CustomTitle
	case s.AITitle != "":
		return s.AITitle
	case s.FirstPrompt != "":
		return s.FirstPrompt
	}
	return s.ShortID()
}

// Named reports whether the user gave this session a name with /rename.
func (s *Session) Named() bool { return s.CustomTitle != "" }

// Repo is the folder name of the working directory.
func (s *Session) Repo() string {
	if s.CWD == "" {
		return "?"
	}
	return filepath.Base(s.CWD)
}

// ShortID is the 8-char prefix Claude Code itself prints.
func (s *Session) ShortID() string {
	if len(s.ID) >= 8 {
		return s.ID[:8]
	}
	return s.ID
}

// Empty is true when nobody ever typed a prompt.
func (s *Session) Empty() bool { return s.Messages == 0 }

// SummaryIdle and SummaryTokens are Claude Code's documented thresholds for the
// "resume from summary" dialog: idle for about an hour and over 100k tokens.
const (
	SummaryIdle   = time.Hour
	SummaryTokens = 100_000
)

// WillOfferSummary predicts whether Claude Code will show the resume-from-summary
// dialog when this session is resumed at time now. It is a prediction: the thresholds
// are approximate, it applies to Pro/Max plans only, and it can't see whether the
// user already chose "Don't ask again".
func (s *Session) WillOfferSummary(now time.Time) bool {
	if s.Live != nil || s.LastActive.IsZero() {
		return false
	}
	return now.Sub(s.LastActive) > SummaryIdle && s.ContextTokens > SummaryTokens
}
