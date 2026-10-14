package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// addSession writes a second transcript, older than recapHome's, into the same project.
func addSession(t *testing.T, home, id string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "projects", "-tmp-repo", id+".jsonl")
	lines := `{"type":"user","message":{"role":"user","content":"rename the tunnel"},"timestamp":"2026-10-14T10:00:00Z","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Renamed."}]},"timestamp":"2026-10-14T10:01:00Z","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeClaude replaces recapHome's claude with body, after a line that counts calls.
func fakeClaude(t *testing.T, home, body string) {
	t.Helper()
	script := "#!/bin/sh\necho call >> " + home + "/calls\n" + body
	if err := os.WriteFile(filepath.Join(home, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func calls(home string) int {
	b, _ := os.ReadFile(filepath.Join(home, "calls"))
	return strings.Count(string(b), "call")
}

const (
	olderID   = "66666666-aaaa-bbbb-cccc-dddddddddddd"
	goodFake  = "cat > /dev/null; echo 'goal  g'; echo 'done  d'; echo 'next  n'\n"
	loginFake = "cat > /dev/null; echo 'Invalid API key. Please authenticate.' >&2; exit 1\n"
)

func TestRecapDryRunListsAndCallsNothing(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	addSession(t, home, olderID)
	fakeClaude(t, home, goodFake)
	cmd := exec.Command(bin, "recap", "--dry-run")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out)
	}
	newer, older := strings.Index(string(out), "77777777"), strings.Index(string(out), "66666666")
	if newer < 0 || older < 0 || newer > older {
		t.Errorf("dry run must list both sessions, newest first: %s", out)
	}
	if n := calls(home); n != 0 {
		t.Errorf("dry run made %d model calls", n)
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("dry run must store nothing")
	}
}

func TestRecapBackfillHonorsLimitAndSkipsCurrent(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	addSession(t, home, olderID)
	fakeClaude(t, home, goodFake)
	run := func(args ...string) string {
		cmd := exec.Command(bin, append([]string{"recap"}, args...)...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("recap %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("--limit", "1")
	b, _ := os.ReadFile(store)
	if !strings.Contains(string(b), "77777777") || strings.Contains(string(b), "66666666") {
		t.Errorf("--limit 1 must recap only the newest: %s", b)
	}
	run()
	if n := calls(home); n != 2 {
		t.Errorf("second run must only recap the one left, calls = %d", n)
	}
	if out := run(); !strings.Contains(out, "every session") {
		t.Errorf("nothing left to do should say so: %s", out)
	}
	if n := calls(home); n != 2 {
		t.Errorf("a current Recap must not be made again, calls = %d", n)
	}
}

func TestRecapBackfillStopsAnAccountAfterAFailedLogin(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	addSession(t, home, olderID)
	fakeClaude(t, home, loginFake)
	cmd := exec.Command(bin, "recap")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("every call failing must exit non-zero: %s", out)
	}
	if n := calls(home); n != 1 {
		t.Errorf("a failed login must skip the account's other sessions, calls = %d\n%s", n, out)
	}
	if strings.Count(string(out), "✗") != 2 {
		t.Errorf("each session gets a line: %s", out)
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("nothing may be stored")
	}
}

func TestDoctorShowsRecaps(t *testing.T) {
	bin := buildBinary(t)
	for _, enabled := range []bool{false, true} {
		home, _, _, env := recapHome(t, enabled)
		addSession(t, home, olderID)
		if enabled {
			fakeClaude(t, home, goodFake)
			cmd := exec.Command(bin, "recap", "--limit", "1")
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("recap: %v\n%s", err, out)
			}
		}
		cmd := exec.Command(bin, "doctor")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		want := "recaps:  off"
		if enabled {
			want = "recaps:  on, 1 of 2 sessions have one"
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("doctor missing %q:\n%s", want, out)
		}
	}
}
