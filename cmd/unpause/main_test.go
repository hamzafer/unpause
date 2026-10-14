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

func TestSearch(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects", "-tmp-repo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(id, role, text, at string) string {
		content, _ := json.Marshal(text)
		if role == "assistant" {
			content, _ = json.Marshal([]map[string]string{{"type": "text", "text": text}})
		}
		return `{"type":"` + role + `","message":{"role":"` + role + `","content":` + string(content) + `},"timestamp":"` + at + `","cwd":"/tmp/repo","sessionId":"` + id + `"}` + "\n"
	}
	for id, body := range map[string]string{
		// 3 hits: tunnel in the prompt, keychain in prompt and reply.
		"aaaaaaaa-0000-0000-0000-000000000000": line("aaaaaaaa-0000-0000-0000-000000000000", "user", "the tunnel needs the keychain", "2026-09-01T10:00:00Z") +
			line("aaaaaaaa-0000-0000-0000-000000000000", "assistant", "keychain is set", "2026-09-01T10:01:00Z"),
		// keychain without tunnel: not a match.
		"bbbbbbbb-0000-0000-0000-000000000000": line("bbbbbbbb-0000-0000-0000-000000000000", "user", "keychain only here", "2026-09-03T10:00:00Z"),
		// 2 hits, newer than aaaaaaaa but fewer hits.
		"cccccccc-0000-0000-0000-000000000000": line("cccccccc-0000-0000-0000-000000000000", "user", "keychain for the tunnel", "2026-09-02T10:00:00Z"),
	} {
		if err := os.WriteFile(filepath.Join(proj, id+".jsonl"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "XDG_CACHE_HOME=", "XDG_CONFIG_HOME=")
	run := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	out := run("search", "tunnel", "keychain")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 2 results of 2 lines each, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "3×  aaaaaaaa") || !strings.Contains(lines[2], "2×  cccccccc") {
		t.Errorf("ranked by hits:\n%s", out)
	}
	if !strings.HasPrefix(lines[1], "       ") || !strings.Contains(lines[1], "the tunnel needs the keychain") {
		t.Errorf("snippet line:\n%s", out)
	}

	if out := run("search", "tunnel", "keychain", "--limit", "1"); strings.Count(out, "\n") != 2 || !strings.Contains(out, "aaaaaaaa") {
		t.Errorf("--limit 1:\n%s", out)
	}
	if out := run("search", `"for the tunnel"`); !strings.Contains(out, "cccccccc") || strings.Contains(out, "aaaaaaaa") {
		t.Errorf("phrase:\n%s", out)
	}

	var rs []struct {
		Session  struct{ ID string }
		Hits     int
		Snippets []string
	}
	if err := json.Unmarshal([]byte(run("search", "tunnel", "keychain", "--json")), &rs); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Session.ID[:8] != "aaaaaaaa" || rs[0].Hits != 3 || len(rs[0].Snippets) != 2 {
		t.Errorf("--json: %+v", rs)
	}

	if out := run("search", "nowhere"); out != "" {
		t.Errorf("no match should print nothing, got %q", out)
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

// TestAutonameApplyNeverWritesCowork: a stale proposals file naming a Cowork session
// must not get that name written into Claude Desktop's transcript.
func TestAutonameApplyNeverWritesCowork(t *testing.T) {
	bin := buildBinary(t)
	home, env := coworkHome(t)
	id := "c1c1c1c1-1111-2222-3333-444444444444"
	transcript := filepath.Join(home, "Library", "Application Support", "Claude", "local-agent-mode-sessions", "org", "user", "local_bbb", ".claude", "projects", "-sessions-x", id+".jsonl")
	// Untitled, so only the Cowork rule can stop the write.
	body := `{"type":"user","message":{"role":"user","content":"translate this letter"},"timestamp":"2026-09-28T10:00:00Z","cwd":"/sessions/x","sessionId":"` + id + `"}` + "\n"
	if err := os.WriteFile(transcript, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(home, ".cache", "unpause")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	ps, _ := json.Marshal([]map[string]string{{"session_id": id, "path": transcript, "name": "Stale name"}})
	if err := os.WriteFile(filepath.Join(cache, "autoname-proposals.json"), ps, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "autoname", "--apply")
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	if after, _ := os.ReadFile(transcript); string(after) != body {
		t.Errorf("autoname --apply wrote into a Cowork transcript:\n%s\noutput: %s", after, out)
	}
}

func TestMCPRefusesBadExcludePattern(t *testing.T) {
	bin := buildBinary(t)
	env := emptyAccountEnv(t)
	home := strings.TrimPrefix(env[len(env)-3], "HOME=")
	xdg := filepath.Join(home, "xdg")
	if err := os.MkdirAll(filepath.Join(xdg, "unpause"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "unpause", "config.toml"), []byte("[mcp]\nexclude = [\"/x/[work\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "mcp")
	cmd.Env = append(env, "XDG_CONFIG_HOME="+xdg)
	cmd.Stdin = strings.NewReader("") // a started server would wait for a client
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "/x/[work") {
		t.Errorf("mcp must refuse a bad exclude pattern and name it, got %v: %s", err, out)
	}
}

func TestSearchQueryKeepsShellQuotedPhrase(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"tunnel", "keychain"}, "tunnel keychain"},
		{[]string{"secure tunnel"}, `"secure tunnel"`},
		{[]string{`"secure tunnel" key`}, `"secure tunnel" key`},
		{[]string{"tunnel"}, "tunnel"},
	} {
		if got := searchQuery(c.args); got != c.want {
			t.Errorf("%q: got %q, want %q", c.args, got, c.want)
		}
	}
}
