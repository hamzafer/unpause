package recap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzafer/unpause/internal/session"
)

const goodReply = "goal  set up the tunnel\ndone  key in the Keychain\nnext  test from ChatGPT\n"

func transcriptSession(t *testing.T) *session.Session {
	t.Helper()
	b, err := os.ReadFile("../provider/claude/testdata/talk.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return &session.Session{ID: "33333333-2222", Path: path, CWD: "/tmp/repo", Messages: 4, FirstPrompt: "Set up the tunnel for the MCP server"}
}

func TestMakeSendsTalkAndRecordsSize(t *testing.T) {
	s := transcriptSession(t)
	var input string
	model := func(_ context.Context, _ *session.Session, in string) (string, error) {
		input = in
		return goodReply, nil
	}
	r, err := Make(context.Background(), s, model)
	if err != nil {
		t.Fatal(err)
	}
	if r.Goal != "set up the tunnel" || r.Done != "key in the Keychain" || r.Next != "test from ChatGPT" {
		t.Errorf("recap = %+v", r)
	}
	st, _ := os.Stat(s.Path)
	if r.Size != st.Size() || r.At.IsZero() {
		t.Errorf("size %d (file %d), at %v", r.Size, st.Size(), r.At)
	}
	if !strings.Contains(input, "Set up the tunnel") || !strings.Contains(input, "the tunnel is up") {
		t.Errorf("input missing talk:\n%s", input)
	}
	for _, not := range []string{"secrets", "security add", "item added", "system line", "metaonly"} {
		if strings.Contains(input, not) {
			t.Errorf("input has %q, which isn't talk:\n%s", not, input)
		}
	}
}

func TestMakeReportsModelAndParseErrors(t *testing.T) {
	s := transcriptSession(t)
	boom := errors.New("claude -p: exit 1")
	if _, err := Make(context.Background(), s, func(context.Context, *session.Session, string) (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Errorf("model error = %v", err)
	}
	if _, err := Make(context.Background(), s, func(context.Context, *session.Session, string) (string, error) { return "sure, here you go", nil }); err == nil {
		t.Error("an unparseable reply must be an error")
	}
}

func TestMakeRefusesSessionsWithoutRecaps(t *testing.T) {
	called := false
	model := func(context.Context, *session.Session, string) (string, error) { called = true; return goodReply, nil }
	for name, mut := range map[string]func(*session.Session){
		"cowork":    func(s *session.Session) { s.Cowork = true },
		"sidechain": func(s *session.Session) { s.Sidechain = true },
		"empty":     func(s *session.Session) { s.Messages = 0 },
	} {
		s := transcriptSession(t)
		mut(s)
		if _, err := Make(context.Background(), s, model); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if called {
		t.Error("the model must not be called for a Session that can't have a Recap")
	}
}
