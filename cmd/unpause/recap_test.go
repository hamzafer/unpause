package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// recapHome builds a temp HOME with one Session and a fake claude that answers with
// whatever is in $HOME/reply. It returns the transcript, the store path and the env.
func recapHome(t *testing.T, enabled bool) (home, transcript, store string, env []string) {
	t.Helper()
	home = t.TempDir()
	id := "77777777-aaaa-bbbb-cccc-dddddddddddd"
	proj := filepath.Join(home, ".claude", "projects", "-tmp-repo")
	transcript = filepath.Join(proj, id+".jsonl")
	lines := `{"type":"user","message":{"role":"user","content":"find padel courts in Oslo"},"timestamp":"2026-10-15T10:00:00Z","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Bærum has showers."}]},"timestamp":"2026-10-15T10:01:00Z","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n"
	fake := filepath.Join(home, "claude")
	script := "#!/bin/sh\ncat > " + home + "/input\ncat " + home + "/reply\n"
	cfg := `claude = "` + fake + `"` + "\n"
	if enabled {
		cfg += "[recap]\nenabled = true\n"
	}
	cfgDir := filepath.Join(home, ".config", "unpause")
	for dir, files := range map[string]map[string]string{
		proj:   {transcript: lines},
		cfgDir: {filepath.Join(cfgDir, "config.toml"): cfg},
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
	store = filepath.Join(home, ".cache", "unpause", "recaps.json")
	env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "XDG_CACHE_HOME=", "XDG_CONFIG_HOME=")
	return home, transcript, store, env
}

func TestRecapOffExplainsHowToEnable(t *testing.T) {
	bin := buildBinary(t)
	_, _, store, env := recapHome(t, false)
	cmd := exec.Command(bin, "recap", "--session", "77777777")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("recap with Recaps off must exit non-zero: %s", out)
	}
	if !strings.Contains(string(out), "[recap]") || !strings.Contains(string(out), "enabled = true") {
		t.Errorf("must say how to turn Recaps on, got: %s", out)
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("nothing may be stored with Recaps off")
	}
}

func TestRecapOneSessionStoresItAndLeavesTheTranscript(t *testing.T) {
	bin := buildBinary(t)
	home, transcript, store, env := recapHome(t, true)
	before, _ := os.ReadFile(transcript)
	if err := os.WriteFile(filepath.Join(home, "reply"), []byte("goal  padel in Oslo\ndone  Bærum has showers\nnext  book a court\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "recap", "--session", "77777777")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("recap failed: %v\n%s", err, out)
	}
	for _, want := range []string{"padel in Oslo", "Bærum has showers", "book a court"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	if in, _ := os.ReadFile(filepath.Join(home, "input")); !strings.Contains(string(in), "find padel courts in Oslo") {
		t.Errorf("model input missing the first prompt:\n%s", in)
	}
	b, err := os.ReadFile(store)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]struct {
		Goal string `json:"goal"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	r, ok := st[transcript]
	if !ok || r.Goal != "padel in Oslo" || r.Size != int64(len(before)) {
		t.Errorf("store = %s", b)
	}
	if after, _ := os.ReadFile(transcript); string(after) != string(before) {
		t.Error("a Recap must never be written into the Transcript")
	}
}

func TestRecapRejectsABadReplyAndStoresNothing(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	if err := os.WriteFile(filepath.Join(home, "reply"), []byte("Sure! This session was about padel.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "recap", "--session", "77777777")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("a bad reply must exit non-zero: %s", out)
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("a bad reply must store nothing")
	}
}

// makeLive marks recapHome's Session as running: a Claude Code sessions file whose pid is
// this test process, which is alive.
func makeLive(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"pid":` + strconv.Itoa(os.Getpid()) + `,"sessionId":"77777777-aaaa-bbbb-cccc-dddddddddddd","status":"busy"}`
	if err := os.WriteFile(filepath.Join(dir, "live.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecapRefusesALiveSession(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	fakeClaude(t, home, "printf 'goal  a\\ndone  b\\nnext  c\\n'\n")
	makeLive(t, home)
	cmd := exec.Command(bin, "recap", "--session", "77777777")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("a Live Session must exit non-zero: %s", out)
	}
	if !strings.Contains(string(out), "running") {
		t.Errorf("must say the session is running, got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "calls")); err == nil {
		t.Error("Haiku must not be asked about a Live Session")
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("nothing may be stored for a Live Session")
	}
}

// The SessionEnd hook's child (it passes --delay) finds the Session still running when
// Claude Code was only cleared or resumed elsewhere: that is a skip, not a failure.
func TestRecapFromTheHookSkipsALiveSession(t *testing.T) {
	bin := buildBinary(t)
	home, _, store, env := recapHome(t, true)
	fakeClaude(t, home, "printf 'goal  a\\ndone  b\\nnext  c\\n'\n")
	makeLive(t, home)
	cmd := exec.Command(bin, "recap", "--session", "77777777", "--delay", "1ms")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("the hook's child must not fail on a Live Session: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "skipped") {
		t.Errorf("must log the skip, got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "calls")); err == nil {
		t.Error("Haiku must not be asked about a Live Session")
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("nothing may be stored for a Live Session")
	}
}
