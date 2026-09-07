package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanBasic(t *testing.T) {
	s, err := ScanFile("testdata/basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("id = %q", s.ID)
	}
	if s.CustomTitle != "login-work" || s.AITitle != "Add login page" {
		t.Errorf("titles = %q / %q", s.CustomTitle, s.AITitle)
	}
	if s.Title() != "login-work" {
		t.Errorf("Title() = %q, want custom title to win", s.Title())
	}
	if s.FirstPrompt != "hey, please add a login page" {
		t.Errorf("first prompt = %q (slash-command noise must be skipped)", s.FirstPrompt)
	}
	if s.CWD != "/tmp/repo" || s.Branch != "feature/login" {
		t.Errorf("cwd/branch = %q / %q", s.CWD, s.Branch)
	}
	if s.Messages != 3 {
		t.Errorf("messages = %d, want 3 (prompt + 2 replies; tool results and commands excluded)", s.Messages)
	}
	if want := time.Date(2026, 9, 1, 10, 0, 1, 0, time.UTC); !s.Created.Equal(want) {
		t.Errorf("created = %v", s.Created)
	}
	if want := time.Date(2026, 9, 1, 10, 2, 0, 0, time.UTC); !s.LastActive.Equal(want) {
		t.Errorf("last active = %v", s.LastActive)
	}
	if len(s.Preview) != 3 || s.Preview[0].Role != "user" || !strings.Contains(s.Preview[2].Text, "login.tsx") {
		t.Errorf("preview = %+v", s.Preview)
	}
	for _, m := range s.Preview {
		if strings.Contains(m.Text, "secret") {
			t.Errorf("thinking leaked into preview: %q", m.Text)
		}
	}
	if s.Sidechain || s.Empty() {
		t.Errorf("sidechain=%v empty=%v", s.Sidechain, s.Empty())
	}
}

func TestScanEmpty(t *testing.T) {
	s, err := ScanFile("testdata/empty.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Empty() {
		t.Errorf("expected empty session, got %d messages", s.Messages)
	}
	if s.ID != "empty" {
		t.Errorf("id = %q, want filename fallback when no message carries a session id", s.ID)
	}
}

func TestScanTolerantOfGarbage(t *testing.T) {
	in := "not json\n{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":\"hi\"},\"timestamp\":\"bad\"}\n{broken\n"
	s, err := Scan(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.Messages != 1 || s.FirstPrompt != "hi" {
		t.Errorf("got %+v", s)
	}
}

func TestRenameAppendsRecordAndReaderSeesIt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	src, _ := os.ReadFile("testdata/basic.jsonl")
	// strip trailing newline to prove Rename handles that
	if err := os.WriteFile(p, []byte(strings.TrimRight(string(src), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Rename(p, "11111111-2222-3333-4444-555555555555", "  renamed via unpause "); err != nil {
		t.Fatal(err)
	}
	s, err := ScanFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.CustomTitle != "renamed via unpause" {
		t.Errorf("custom title after rename = %q", s.CustomTitle)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasSuffix(string(b), "\"type\":\"custom-title\"}\n") && !strings.Contains(string(b), `{"customTitle":"renamed via unpause","sessionId":"11111111-2222-3333-4444-555555555555","type":"custom-title"}`) {
		t.Errorf("unexpected tail: %q", string(b[len(b)-120:]))
	}
	if strings.Count(string(b), "\n") != strings.Count(string(src), "\n")+1 {
		t.Errorf("rename must add exactly one line")
	}
	if Rename(p, "x", "   ") == nil {
		t.Error("blank name must fail")
	}
}

func TestListTranscriptsSkipsSubagentDirs(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "-tmp-repo")
	os.MkdirAll(filepath.Join(proj, "abc", "subagents"), 0o755)
	os.WriteFile(filepath.Join(proj, "abc.jsonl"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "abc", "subagents", "agent-1.jsonl"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "sessions-index.json"), []byte("{}"), 0o644)
	ts, err := ListTranscripts(rootFor(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || !strings.HasSuffix(ts[0].Path, "abc.jsonl") {
		t.Errorf("got %+v", ts)
	}
}
