package autoname

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzafer/unpause/internal/session"
)

func TestNeeds(t *testing.T) {
	cases := []struct {
		name string
		s    session.Session
		want bool
	}{
		{"untitled", session.Session{Messages: 3, FirstPrompt: "fix the bug"}, true},
		{"named by user", session.Session{Messages: 3, CustomTitle: "hertz tracker"}, false},
		{"auto titled by Claude", session.Session{Messages: 3, AITitle: "Add login page"}, false},
		{"empty", session.Session{}, false},
		{"subagent", session.Session{Messages: 3, Sidechain: true}, false},
	}
	for _, c := range cases {
		if got := Needs(&c.s); got != c.want {
			t.Errorf("%s: Needs = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"Automatic Haiku Session Naming":        "Automatic Haiku Session Naming",
		`"Fix login redirect."`:                 "Fix login redirect",
		"**Supabase grants review**":            "Supabase grants review",
		"Name: Bike search in Oslo":             "Bike search in Oslo",
		"\n\n  Title:  `Warp tab opener`  \nok": "Warp tab opener",
		"   ":                                   "",
		strings.Repeat("word ", 30):             "word word word word word word word word word word word word",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPromptCarriesContext(t *testing.T) {
	s := &session.Session{
		CWD:         "/Users/x/developer/unpause",
		FirstPrompt: "can we rename sessions automatically",
		Preview: []session.Message{
			{Role: "user", Text: "use haiku please"},
			{Role: "assistant", Text: "Sure, a SessionEnd hook."},
		},
	}
	p := Prompt(s)
	for _, want := range []string{"unpause", "can we rename sessions automatically", "use haiku please", "SessionEnd hook"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestRunDryRunNeverWrites(t *testing.T) {
	sessions := []*session.Session{{ID: "a", Messages: 1}, {ID: "b", Messages: 1}}
	namer := func(ctx context.Context, s *session.Session) (string, error) { return `"Name for ` + s.ID + `"`, nil }
	write := func(s *session.Session, name string) error {
		t.Fatalf("dry run wrote %q to %s", name, s.ID)
		return nil
	}
	var got []Result
	Run(context.Background(), sessions, namer, write, true, func(r Result) { got = append(got, r) })
	if len(got) != 2 || got[0].Name != "Name for a" || got[1].Name != "Name for b" {
		t.Fatalf("unexpected results: %+v", got)
	}
}

func TestRunWritesCleanNamesAndReportsErrors(t *testing.T) {
	sessions := []*session.Session{{ID: "ok", Messages: 1}, {ID: "blank", Messages: 1}, {ID: "fail", Messages: 1}}
	namer := func(ctx context.Context, s *session.Session) (string, error) {
		switch s.ID {
		case "blank":
			return "  ", nil
		case "fail":
			return "", errors.New("boom")
		}
		return "Good name.", nil
	}
	written := map[string]string{}
	write := func(s *session.Session, name string) error { written[s.ID] = name; return nil }
	errs := 0
	Run(context.Background(), sessions, namer, write, false, func(r Result) {
		if r.Err != nil {
			errs++
		}
	})
	if len(written) != 1 || written["ok"] != "Good name" {
		t.Errorf("written = %v, want only ok → Good name", written)
	}
	if errs != 2 {
		t.Errorf("errors reported = %d, want 2 (blank name and namer failure)", errs)
	}
}

func TestReadHookInput(t *testing.T) {
	in := `{"session_id":"abc123","transcript_path":"/tmp/x/abc123.jsonl","cwd":"/tmp","hook_event_name":"SessionEnd","reason":"other"}`
	h, err := ReadHookInput(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if h.SessionID != "abc123" || h.TranscriptPath != "/tmp/x/abc123.jsonl" || h.Reason != "other" {
		t.Errorf("parsed %+v", h)
	}
	if _, err := ReadHookInput(strings.NewReader(`{"reason":"other"}`)); err == nil {
		t.Error("expected an error when session_id is missing")
	}
}

func TestChildEnv(t *testing.T) {
	base := []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/leak"}
	if env := childEnv(base, ""); strings.Contains(strings.Join(env, " "), "CLAUDE_CONFIG_DIR") {
		t.Errorf("default account must unset CLAUDE_CONFIG_DIR, got %v", env)
	}
	env := strings.Join(childEnv(base, "/Users/x/.claude-work"), " ")
	if !strings.Contains(env, "CLAUDE_CONFIG_DIR=/Users/x/.claude-work") || strings.Contains(env, "/leak") {
		t.Errorf("other accounts must get their own dir, got %v", env)
	}
	if !strings.Contains(env, "UNPAUSE_AUTONAME=1") {
		t.Errorf("child must carry the recursion guard, got %v", env)
	}
}

// fakeClaude writes a shell script standing in for the claude binary. It logs each call
// (args, CLAUDE_CONFIG_DIR, stdin) and fails to authenticate for the "/dead" account.
func fakeClaude(t *testing.T) (bin, calls string) {
	t.Helper()
	dir := t.TempDir()
	bin, calls = filepath.Join(dir, "claude"), filepath.Join(dir, "calls")
	script := `#!/bin/sh
echo "CALL dir=$CLAUDE_CONFIG_DIR guard=$UNPAUSE_AUTONAME args=$*" >> "` + calls + `"
cat >> "` + calls + `"
if [ "$CLAUDE_CONFIG_DIR" = "/dead" ]; then echo "Failed to authenticate: OAuth session expired" >&2; exit 1; fi
echo '"Fake Session Name."'
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, calls
}

func TestHaikuCallsClaudeAndSkipsLoggedOutAccounts(t *testing.T) {
	bin, calls := fakeClaude(t)
	namer := Haiku(bin, func(s *session.Session) string { return s.Root })
	ctx := context.Background()

	name, err := namer(ctx, &session.Session{Root: "/work", FirstPrompt: "wire up the hook"})
	if err != nil || Clean(name) != "Fake Session Name" {
		t.Fatalf("namer = %q, %v", name, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := namer(ctx, &session.Session{Root: "/dead"}); err == nil || !strings.Contains(err.Error(), "authenticate") {
			t.Fatalf("dead account call %d: err = %v", i, err)
		}
	}

	log, _ := os.ReadFile(calls)
	got := string(log)
	for _, want := range []string{"--model haiku", "--no-session-persistence", "disableAllHooks", "guard=1", "dir=/work", "wire up the hook"} {
		if !strings.Contains(got, want) {
			t.Errorf("call log missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "CALL dir=/dead"); n != 1 {
		t.Errorf("logged-out account was called %d times, want 1", n)
	}
}
