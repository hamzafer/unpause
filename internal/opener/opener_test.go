package opener

import (
	"strings"
	"testing"
)

func TestShellCommandQuotes(t *testing.T) {
	l := Launch{Claude: "claude", SessionID: "abc", CWD: "/Users/me/my repo", ConfigDir: "/Users/me/.claude-work", Fork: true}
	got := l.ShellCommand()
	want := `cd '/Users/me/my repo' && CLAUDE_CONFIG_DIR=/Users/me/.claude-work exec claude --resume abc --fork-session`
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestPickUnknown(t *testing.T) {
	if _, err := Pick("carrier-pigeon"); err == nil {
		t.Error("expected error")
	}
}

func TestAsQuoteEscapes(t *testing.T) {
	if got := asQuote(`say "hi" \ bye`); got != `"say \"hi\" \\ bye"` {
		t.Error(got)
	}
	if !strings.HasPrefix(tmuxName("a.b:c"), "a-b-c") {
		t.Error(tmuxName("a.b:c"))
	}
}
