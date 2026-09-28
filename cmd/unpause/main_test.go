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
