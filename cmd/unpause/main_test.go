package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestMCPStdoutIsOnlyProtocol(t *testing.T) {
	bin := buildBinary(t)
	env := emptyAccountEnv(t)
	home := strings.TrimPrefix(env[len(env)-3], "HOME=")
	proj := filepath.Join(home, ".claude", "projects", "-tmp-repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../internal/provider/claude/testdata/basic.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(proj, "11111111-2222-3333-4444-555555555555.jsonl")
	if err := os.WriteFile(transcript, src, 0o644); err != nil {
		t.Fatal(err)
	}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_sessions","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_session","arguments":{"id":"1111","offset":1,"limit":1}}}`,
	}, "\n") + "\n"
	cmd := exec.Command(bin, "mcp")
	cmd.Env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"))
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, in); err != nil {
		t.Fatal(err)
	}
	// Every line on stdout must be a JSON-RPC message; the three requests get three replies.
	var lines []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(nil, 1<<20)
	for len(lines) < 3 && sc.Scan() {
		l := sc.Text()
		var msg map[string]any
		if err := json.Unmarshal([]byte(l), &msg); err != nil || msg["jsonrpc"] != "2.0" {
			t.Errorf("non-protocol line on stdout: %q", l)
		}
		if _, ok := msg["id"]; ok {
			lines = append(lines, l)
		}
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Errorf("unpause mcp exit: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("want 3 responses, got %d: %q", len(lines), lines)
	}
	sort.Strings(lines) // replies may come in any order; ids 1, 2, 3 sort by their prefix
	if !strings.Contains(lines[1], "login-work") || !strings.Contains(lines[2], "hey, please add a login page") {
		t.Errorf("unexpected responses: %q", lines)
	}
	after, _ := os.ReadFile(transcript)
	if string(after) != string(src) {
		t.Error("transcript changed")
	}
}
