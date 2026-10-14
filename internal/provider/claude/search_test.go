package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

func TestParseTerms(t *testing.T) {
	for q, want := range map[string][]string{
		"":                      nil,
		"   ":                   nil,
		`""`:                    nil,
		"Tunnel  Keychain":      {"tunnel", "keychain"},
		`"runtime key" tunnel`:  {"runtime key", "tunnel"},
		`foo "the tunnel is`:    {"foo", "the tunnel is"},
		`a "b  c" d`:            {"a", "b  c", "d"},
		"tunnel TUNNEL tunnel":  {"tunnel"},
		"\tkey\nchain ":         {"key", "chain"},
		`C:\Users`:              {`c:\users`},
		`"  spaced phrase  " x`: {"spaced phrase", "x"},
	} {
		if got := NewMatcher(q).Terms(); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: terms = %q, want %q", q, got, want)
		}
	}
}

func search(t *testing.T, q, path string) *SearchResult {
	t.Helper()
	r, err := NewMatcher(q).SearchFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSearchFileTalkOnly(t *testing.T) {
	// "keychain": the reply "...in the Keychain." and "Done: the keychain...". Not the
	// thinking, the tool result or the system line.
	r := search(t, "keychain", "testdata/talk.jsonl")
	if r.Hits != 2 || !r.All() || len(r.Snippets) != 2 {
		t.Fatalf("keychain: %+v", r)
	}
	if !strings.Contains(r.Snippets[0], "runtime key in the Keychain.") || !strings.Contains(r.Snippets[1], "the keychain holds") {
		t.Errorf("snippets = %q", r.Snippets)
	}
	for _, q := range []string{
		"metaonly",      // isMeta (injected) user record only
		"secrets",       // thinking only
		"security",      // tool_use input only
		"bash",          // tool name only
		`"item added"`,  // tool_result only
		`"system line"`, // not a message
		`"where is it"`, // thinking in blocks.jsonl
		"permission",    // tool_result in blocks.jsonl
		"grep",          // tool name in blocks.jsonl
	} {
		for _, p := range []string{"testdata/talk.jsonl", "testdata/blocks.jsonl"} {
			if r := search(t, q, p); r.Hits != 0 || r.All() || len(r.Snippets) != 0 {
				t.Errorf("%q in %s must not match: %+v", q, p, r)
			}
		}
	}
	// blocks.jsonl: "Needle" in the prompt counts; tool input and tool result don't.
	r = search(t, "needle", "testdata/blocks.jsonl")
	if r.Hits != 1 || len(r.Snippets) != 1 || r.Snippets[0] != "Find the Needle in config" {
		t.Errorf("needle: %+v", r)
	}
}

func TestSearchFileEveryTerm(t *testing.T) {
	// "tunnel" is in the first prompt and the last reply, "runtime" only in the first reply.
	r := search(t, "runtime tunnel", "testdata/talk.jsonl")
	if !r.All() || r.Hits != 3 {
		t.Errorf("terms across messages: %+v", r)
	}
	r = search(t, "tunnel nowhere", "testdata/talk.jsonl")
	if r.All() || r.Hits != 2 || !r.Seen[0] || r.Seen[1] {
		t.Errorf("one term missing: %+v", r)
	}
	// Snippets in file order, up to 3; a match inside the previous snippet's window adds none.
	r = search(t, "keychain tunnel", "testdata/talk.jsonl")
	if r.Hits != 4 || len(r.Snippets) != 3 {
		t.Fatalf("keychain tunnel: %+v", r)
	}
	if !strings.HasPrefix(r.Snippets[0], "Set up the tunnel") || !strings.Contains(r.Snippets[1], "Keychain") || !strings.Contains(r.Snippets[2], "keychain holds the key, the tunnel is up") {
		t.Errorf("snippet order: %q", r.Snippets)
	}
}

func TestSearchFilePhrases(t *testing.T) {
	if r := search(t, `"runtime key"`, "testdata/talk.jsonl"); r.Hits != 1 || !r.All() {
		t.Errorf("phrase: %+v", r)
	}
	if r := search(t, `"key runtime"`, "testdata/talk.jsonl"); r.Hits != 0 || r.All() {
		t.Errorf("words in the wrong order are not the phrase: %+v", r)
	}
	if r := search(t, `"the tunnel is`, "testdata/talk.jsonl"); r.Hits != 1 || !r.All() {
		t.Errorf("unclosed quote runs to the end: %+v", r)
	}
	// JSON escaping in the raw-line pre-filter: a backslash is stored as \\.
	if r := search(t, `c:\users\path`, "testdata/talk.jsonl"); r.Hits != 1 {
		t.Errorf("backslash term: %+v", r)
	}
	// A phrase spanning a newline in the stored text: whitespace is not normalized.
	if r := search(t, `"and spans"`, "testdata/blocks.jsonl"); r.Hits != 0 {
		t.Errorf("newline is not a space: %+v", r)
	}
	if r := search(t, "quoted spans", "testdata/blocks.jsonl"); r.Hits != 2 || !r.All() {
		t.Errorf("text with escaped quotes and newlines: %+v", r)
	}
}

func TestSearchFileEmptyAndCancel(t *testing.T) {
	if r := search(t, "  ", "testdata/talk.jsonl"); r.Hits != 0 || r.All() {
		t.Errorf("empty query matches nothing: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewMatcher("tunnel").SearchFile(ctx, "testdata/talk.jsonl"); err == nil {
		t.Error("cancelled context must stop the scan")
	}
}

func TestSnippetWindow(t *testing.T) {
	text := strings.Repeat("a ", 200) + "TARGET" + strings.Repeat(" b", 200)
	m := NewMatcher("target")
	r := m.newResult()
	m.match(text, r)
	if r.Hits != 1 || len(r.Snippets) != 1 {
		t.Fatalf("%+v", r)
	}
	s := r.Snippets[0]
	if !strings.Contains(s, "TARGET") || len([]rune(s)) > 210 || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Errorf("snippet = %q (%d runes)", s, len([]rune(s)))
	}
}

func TestMatchTitle(t *testing.T) {
	m := NewMatcher("keychain setup")
	r := m.newResult()
	m.MatchTitle("Keychain setup for the keychain", r)
	if r.Hits != 3 || !r.All() || len(r.Snippets) != 0 {
		t.Errorf("title: %+v", r)
	}
}

func shortIDs(rs []Result) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Session.ID)
	}
	return strings.Join(out, ",")
}

func TestSearchRanksAndFilters(t *testing.T) {
	dir := t.TempDir()
	many := filepath.Join(dir, "many.jsonl")
	line := `{"type":"user","message":{"role":"user","content":"keychain keychain keychain tunnel tunnel"},"timestamp":"2026-09-03T10:00:00.000Z","sessionId":"m"}` + "\n"
	if err := os.WriteFile(many, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ss := []*session.Session{
		{ID: "old", Path: "testdata/talk.jsonl", LastActive: now.Add(-2 * time.Hour)},
		{ID: "many", Path: many, LastActive: now.Add(-5 * time.Hour)},
		{ID: "new", Path: "testdata/talk.jsonl", LastActive: now},
		{ID: "basic", Path: "testdata/basic.jsonl", LastActive: now.Add(time.Hour)},
		{ID: "gone", Path: filepath.Join(dir, "missing.jsonl"), LastActive: now},
		// Only the title says keychain; a Name counts.
		{ID: "titled", Path: "testdata/basic.jsonl", CustomTitle: "Keychain notes", LastActive: now.Add(-time.Hour)},
		// A Title made from the first prompt is not counted.
		{ID: "prompt", Path: "testdata/basic.jsonl", FirstPrompt: "keychain tunnel", LastActive: now},
	}
	var mu sync.Mutex
	var calls, lastDone, lastTotal int
	rs, err := Search(context.Background(), ss, "keychain tunnel", func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if done < lastDone {
			t.Errorf("progress went backwards: %d after %d", done, lastDone)
		}
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	// many: 3+2 hits. new and old: 2+2 each, newest first. titled has no tunnel.
	if got := shortIDs(rs); got != "many,new,old" {
		t.Errorf("ranked = %s", got)
	}
	if rs[0].Hits != 5 || rs[1].Hits != 4 || len(rs[1].Snippets) != 3 {
		t.Errorf("hits/snippets: %+v", rs)
	}
	if calls != len(ss) || lastDone != len(ss) || lastTotal != len(ss) {
		t.Errorf("progress: calls=%d last=%d/%d", calls, lastDone, lastTotal)
	}

	rs, _ = Search(context.Background(), ss, "keychain login", nil)
	// The Name (1) plus "login page" and "login.tsx" in basic.jsonl's talk (2). The
	// Name "login-work" in basic.jsonl is not this Session's title, so it doesn't count.
	if got := shortIDs(rs); got != "titled" || rs[0].Hits != 3 || len(rs[0].Snippets) != 2 {
		t.Errorf("title and talk together: %s %+v", got, rs)
	}
}

func TestSearchEmptyQueryAndCancel(t *testing.T) {
	ss := []*session.Session{{ID: "a", Path: "testdata/talk.jsonl"}}
	for _, q := range []string{"", "   ", `""`} {
		rs, err := Search(context.Background(), ss, q, nil)
		if err != nil || len(rs) != 0 {
			t.Errorf("%q: %v %+v", q, err, rs)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, ss, "tunnel", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: err = %v", err)
	}
}

func TestSearchFileAlternateEscapesAndControlChars(t *testing.T) {
	path := filepath.Join(t.TempDir(), "esc.jsonl")
	lines := `{"type":"user","message":{"role":"user","content":"open the tunnel at a\/b"},"timestamp":"2026-09-03T10:00:00.000Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"colored \u001b[31mwidget\u001b[0m output"}]},"timestamp":"2026-09-03T10:00:01.000Z"}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := search(t, "tunnel", path); r.Hits != 1 {
		t.Errorf(`\u escape: %+v`, r)
	}
	if r := search(t, "a/b", path); r.Hits != 1 {
		t.Errorf(`\/ escape: %+v`, r)
	}
	r := search(t, "widget", path)
	if r.Hits != 1 || strings.ContainsRune(r.Snippets[0], '\x1b') {
		t.Errorf("control chars: %+v", r)
	}
}

func TestIsMetaReadsTheTopLevelField(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"type":"user","isMeta":true,"message":{"content":"x"}}`:                                                 true,
		`{"type":"user", "isMeta": true, "message":{"content":"x"}}`:                                              true,
		`{"type":"user","isMeta":false,"message":{"content":"x"}}`:                                                false,
		`{"type":"user","message":{"content":"the field \"isMeta\":true in JSON"}}`:                               false,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"isMeta":true}}]}}`: false,
	} {
		if got := isMeta([]byte(raw)); got != want {
			t.Errorf("%s: got %v", raw, got)
		}
	}
}

func TestTalkDropsWrappersAndInjectedText(t *testing.T) {
	for in, want := range map[string]string{
		"plain words": "plain words",
		"<command-name>/goal</command-name>\n<command-message>goal</command-message>\n<command-args>find padel in oslo</command-args>": "/goal find padel in oslo",
		"<command-message>loop</command-message>\n<command-name>/loop</command-name>":                                                  "/loop",
		"<bash-input> ls -la</bash-input>": "! ls -la",
		"see the <pasted_content> below":   "see the <pasted_content> below",
	} {
		if got, ok := talk(in); !ok || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"<local-command-stdout>Goal set: find padel</local-command-stdout>",
		"<local-command-caveat>Caveat: generated</local-command-caveat>",
		"\n<task-notification>\n<task-id>x</task-id> padel report</task-notification>",
		"<bash-stdout>padel</bash-stdout><bash-stderr></bash-stderr>",
		"<system-reminder>You are in a worktree</system-reminder>",
		"<fork-boilerplate>You are a worker fork</fork-boilerplate>",
		"<command-message>reviews the current diff</command-message>",
	} {
		if _, ok := talk(in); ok {
			t.Errorf("%q should not be talk", in)
		}
	}
}

func TestTalkKeepsProseThatMentionsTags(t *testing.T) {
	in := "the record stores <command-name>/goal</command-name> next to the args"
	if got, ok := talk(in); !ok || got != in {
		t.Errorf("got %q, %v", got, ok)
	}
}

func TestSearchFileCountsASlashCommandOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cmd.jsonl")
	line := func(content string) string {
		// Claude Code writes JSON without escaping <, > and &, so the fixture doesn't either.
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(content)
		return `{"type":"user","message":{"role":"user","content":` + strings.TrimSpace(buf.String()) + `},"timestamp":"2026-09-03T10:00:00.000Z"}` + "\n"
	}
	body := line("<command-name>/goal</command-name>\n<command-message>goal</command-message>\n<command-args>find padel courts</command-args>") +
		line("<local-command-stdout>Goal set: find padel courts</local-command-stdout>")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := search(t, "padel", path)
	if r.Hits != 1 || len(r.Snippets) != 1 || r.Snippets[0] != "/goal find padel courts" {
		t.Errorf("%+v", r)
	}
	// The phrase only exists once the tags are gone; the raw-line pre-filter must not skip it.
	if r := search(t, `"/goal find padel"`, path); r.Hits != 1 {
		t.Errorf("phrase across the command name: %+v", r)
	}
	// Claude's replies are never unwrapped or dropped.
	reply := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"<bash-stdout> is where padel output goes"}]},"timestamp":"2026-09-03T10:00:01.000Z"}` + "\n"
	if err := os.WriteFile(path, []byte(body+reply), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := search(t, "padel", path); r.Hits != 2 {
		t.Errorf("assistant text: %+v", r)
	}
}
