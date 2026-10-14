package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the real CLI once per test run; main.go wires cobra flags
// directly onto closures, so exercising it as a subprocess is simpler than unit-testing internals.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/unpause"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestOpenHelpDocumentsJSONFlag(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "open", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, want := range []string{"--print", "--json", "--fork", "--open"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("open --help missing %q\n%s", want, out)
		}
	}
}

func TestOpenUnknownSessionErrorsCleanly(t *testing.T) {
	bin := buildBinary(t)
	// Give the subprocess its own empty account so the failure we're testing is
	// "no session matches", not "no account found" (which is what a bare CI runner
	// with no ~/.claude at all would hit instead).
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "open", "definitely-not-a-real-session-id-xyz", "--print", "--json")
	cmd.Env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit for an unmatched id, got success: %s", out)
	}
	if !strings.Contains(string(out), "no session matches") {
		t.Errorf("expected a clear error, got: %s", out)
	}
}

// emptyAccountEnv points the subprocess at a fresh, empty ~/.claude.
func emptyAccountEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "XDG_CACHE_HOME="+filepath.Join(home, "cache"))
}

func TestAutonameWithNothingToDo(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "autoname", "--dry-run")
	cmd.Env = emptyAccountEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("autoname --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "already has a name") {
		t.Errorf("expected the nothing-to-do message, got: %s", out)
	}
}

func TestSessionEndHookGuardAndBadInput(t *testing.T) {
	bin := buildBinary(t)
	env := emptyAccountEnv(t)

	// Inside a naming call the hook must do nothing, even with garbage on stdin.
	guarded := exec.Command(bin, "hook", "session-end")
	guarded.Env = append(env, "UNPAUSE_AUTONAME=1")
	guarded.Stdin = strings.NewReader("not json")
	if out, err := guarded.CombinedOutput(); err != nil {
		t.Errorf("guarded hook should exit 0, got %v: %s", err, out)
	}

	bad := exec.Command(bin, "hook", "session-end")
	bad.Env = env
	bad.Stdin = strings.NewReader(`{"reason":"other"}`)
	if out, err := bad.CombinedOutput(); err == nil {
		t.Errorf("hook input without session_id should fail, got success: %s", out)
	}
}

// TestAutonameDryRunThenApply checks that --apply writes the name the dry run showed,
// even when the model would answer differently the second time.
func TestAutonameDryRunThenApply(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	id := "99999999-aaaa-bbbb-cccc-dddddddddddd"
	proj := filepath.Join(home, ".claude", "projects", "-tmp-repo")
	transcript := filepath.Join(proj, id+".jsonl")
	lines := `{"type":"user","message":{"role":"user","content":"add a dark mode toggle"},"timestamp":"2026-09-28T10:00:00Z","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n"
	// The fake claude answers with a counter, so a second call would give a different name.
	fake := filepath.Join(home, "claude")
	script := "#!/bin/sh\ncat >/dev/null\nn=$(cat " + home + "/n 2>/dev/null || echo 0); n=$((n+1)); echo $n > " + home + "/n\necho \"Dark mode take $n\"\n"
	cfgDir := filepath.Join(home, ".config", "unpause")
	for dir, files := range map[string]map[string]string{
		proj:   {transcript: lines},
		cfgDir: {filepath.Join(cfgDir, "config.toml"): `claude = "` + fake + `"` + "\n"},
		home:   {fake: script},
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for p, body := range files {
			if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	env := append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "XDG_CACHE_HOME=", "XDG_CONFIG_HOME=")
	run := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	if out := run("autoname", "--dry-run"); !strings.Contains(out, "Dark mode take 1") {
		t.Fatalf("dry run output: %s", out)
	}
	if b, _ := os.ReadFile(transcript); strings.Contains(string(b), "custom-title") {
		t.Fatal("dry run wrote to the transcript")
	}
	// A write that fails keeps its name saved so the next --apply can retry it.
	if err := os.Chmod(transcript, 0o444); err != nil {
		t.Fatal(err)
	}
	if out := run("autoname", "--apply"); !strings.Contains(out, "kept 1") {
		t.Fatalf("failed apply should keep the proposal: %s", out)
	}
	if err := os.Chmod(transcript, 0o644); err != nil {
		t.Fatal(err)
	}
	run("autoname", "--apply")
	b, _ := os.ReadFile(transcript)
	if !strings.Contains(string(b), `"customTitle":"Dark mode take 1"`) {
		t.Errorf("apply should write the dry-run name, transcript:\n%s", b)
	}
	cmd := exec.Command(bin, "autoname", "--apply")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "--dry-run` first") {
		t.Errorf("second apply should say to dry-run first, got %v: %s", err, out)
	}

	// Now that the session is named, a new dry run finds nothing, and that must also
	// clear any proposals left from before so --apply can't write stale names.
	if err := os.WriteFile(filepath.Join(home, ".cache", "unpause", "autoname-proposals.json"), []byte(`[{"session_id":"x","path":"/nope","name":"stale"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	run("autoname", "--dry-run")
	cmd = exec.Command(bin, "autoname", "--apply")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil || strings.Contains(string(out), "stale") {
		t.Errorf("an empty dry run should clear stale proposals, got %v: %s", err, out)
	}

	cmd = exec.Command(bin, "autoname", "--session", id, "--dry-run")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("--session with --dry-run should be refused: %s", out)
	}
}

// coworkHome builds a temp HOME with two Cowork sessions laid out like Claude Desktop does:
// one whose metadata has a title and one whose metadata title is empty.
func coworkHome(t *testing.T) (home string, env []string) {
	t.Helper()
	home = t.TempDir()
	base := filepath.Join(home, "Library", "Application Support", "Claude", "local-agent-mode-sessions", "org", "user")
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ local, id, title, prompt string }{
		{"local_aaa", "c0c0c0c0-1111-2222-3333-444444444444", "Build metrics dashboard", "make me a dashboard"},
		{"local_bbb", "c1c1c1c1-1111-2222-3333-444444444444", "", "translate this letter"},
	} {
		write(filepath.Join(base, c.local+".json"), `{"sessionId":"`+c.local+`","cliSessionId":"`+c.id+`","title":"`+c.title+`","createdAt":1769460665209,"lastActivityAt":1769460775175,"isArchived":false,"cwd":"/sessions/x"}`)
		write(filepath.Join(base, c.local, ".claude", "projects", "-sessions-x", c.id+".jsonl"),
			`{"type":"user","message":{"role":"user","content":"`+c.prompt+`"},"timestamp":"2026-09-28T10:00:00Z","cwd":"/sessions/x","sessionId":"`+c.id+`"}`+"\n"+
				`{"type":"ai-title","aiTitle":"Generated `+c.prompt+`","sessionId":"`+c.id+`"}`+"\n")
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "XDG_CACHE_HOME=", "XDG_CONFIG_HOME=")
}

func TestCoworkSessionsListedButReadOnly(t *testing.T) {
	bin := buildBinary(t)
	home, env := coworkHome(t)
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run("list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	for _, want := range []string{"c0c0c0c0 cowork", "Build metrics dashboard", "c1c1c1c1 cowork", "Generated translate this letter"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "(cwd missing)") {
		t.Errorf("a Cowork Session's VM cwd must not be flagged as missing\n%s", out)
	}
	if strings.Contains(out, "Generated make me a dashboard") {
		t.Errorf("Claude Desktop's title must win over the transcript's ai-title\n%s", out)
	}

	out, err = run("list", "--json")
	if err != nil || !strings.Contains(out, `"cowork": true`) || !strings.Contains(out, `"desktop_title": "Build metrics dashboard"`) {
		t.Errorf("list --json: %v\n%s", err, out)
	}

	for _, args := range [][]string{{"open", "c0c0c0c0"}, {"open", "c0c0c0c0", "--print"}} {
		if out, err := run(args...); err == nil || !strings.Contains(out, "cowork session: open it in Claude Desktop") {
			t.Errorf("%v should refuse, got %v: %s", args, err, out)
		}
	}
	transcript := filepath.Join(home, "Library", "Application Support", "Claude", "local-agent-mode-sessions", "org", "user", "local_bbb", ".claude", "projects", "-sessions-x", "c1c1c1c1-1111-2222-3333-444444444444.jsonl")
	before, _ := os.ReadFile(transcript)
	if out, err := run("rename", "c1c1c1c1", "new name"); err == nil || !strings.Contains(out, "cowork session") {
		t.Errorf("rename should refuse, got %v: %s", err, out)
	}
	if after, _ := os.ReadFile(transcript); string(after) != string(before) {
		t.Error("rename wrote into a Cowork transcript")
	}

	out, err = run("doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cowork     2 roots, 2 transcripts") || strings.Contains(out, "local_aaa") {
		t.Errorf("doctor should collapse cowork roots into one line\n%s", out)
	}
}
