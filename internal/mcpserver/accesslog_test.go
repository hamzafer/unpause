package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hamzafer/unpause/internal/session"
)

// inputOf decodes an Entry's input into a map for comparison.
func inputOf(t *testing.T, e Entry) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Input, &m); err != nil {
		t.Fatalf("input %s: %v", e.Input, err)
	}
	return m
}

func TestAccessLogPerTool(t *testing.T) {
	e := setup(t)
	var l ListOutput
	e.call(t, "list_sessions", map[string]any{"account": "personal", "project": "dev", "live": false, "limit": 2, "offset": 1}, &l)
	var s SearchOutput
	e.call(t, "search_sessions", map[string]any{"query": "secret", "account": "work", "limit": 2}, &s)
	var g GetOutput
	e.call(t, "get_session", map[string]any{"id": "2222", "offset": 1, "limit": 2, "max_chars": 5}, &g)

	got := e.log.all()
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	for i, tool := range []string{"list_sessions", "search_sessions", "get_session"} {
		en := got[i]
		if en.Tool != tool {
			t.Errorf("entry %d tool = %q, want %q", i, en.Tool, tool)
		}
		if en.Client != "test-client" || en.ClientVersion != "0" {
			t.Errorf("%s client = %q %q", tool, en.Client, en.ClientVersion)
		}
		at, err := time.Parse(time.RFC3339, en.At)
		if err != nil || at.Location() != time.UTC || time.Since(at) > time.Minute {
			t.Errorf("%s at = %q (%v)", tool, en.At, err)
		}
		if en.DurationMS < 0 || en.Error != "" {
			t.Errorf("%s duration=%d error=%q", tool, en.DurationMS, en.Error)
		}
	}

	list := got[0]
	if want := map[string]any{"account": "personal", "project": "dev", "live": false, "limit": 2.0, "offset": 1.0}; !reflect.DeepEqual(inputOf(t, list), want) {
		t.Errorf("list input = %v, want %v", inputOf(t, list), want)
	}
	var wantIDs []string
	for _, x := range l.Sessions {
		wantIDs = append(wantIDs, x.ID)
	}
	if !reflect.DeepEqual(list.SessionIDs, wantIDs) || list.Count != len(l.Sessions) || list.Total != l.Total {
		t.Errorf("list ids=%v count=%d total=%d, want %v %d %d", list.SessionIDs, list.Count, list.Total, wantIDs, len(l.Sessions), l.Total)
	}

	search := got[1]
	if want := map[string]any{"query": "secret", "account": "work", "limit": 2.0}; !reflect.DeepEqual(inputOf(t, search), want) {
		t.Errorf("search input = %v, want %v", inputOf(t, search), want)
	}
	wantIDs = nil
	for _, x := range s.Results {
		wantIDs = append(wantIDs, x.ID)
	}
	if !reflect.DeepEqual(search.SessionIDs, wantIDs) || search.Count != 2 || search.Total != s.Total || s.Total != 3 {
		t.Errorf("search ids=%v count=%d total=%d, want %v 2 %d", search.SessionIDs, search.Count, search.Total, wantIDs, s.Total)
	}

	get := got[2]
	if want := map[string]any{"id": "2222", "offset": 1.0, "limit": 2.0, "max_chars": 5.0}; !reflect.DeepEqual(inputOf(t, get), want) {
		t.Errorf("get input = %v, want %v", inputOf(t, get), want)
	}
	if !reflect.DeepEqual(get.SessionIDs, []string{"22222222-2222-3333-4444-555555555555"}) || get.Count != 2 || get.Total != 5 {
		t.Errorf("get ids=%v count=%d total=%d", get.SessionIDs, get.Count, get.Total)
	}
}

func TestAccessLogErrors(t *testing.T) {
	e := setup(t, "/x/dev/secret-repo")
	for _, args := range []map[string]any{{"id": "ffff"}, {"id": "cccc0005"}} {
		res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: args})
		if err != nil || !res.IsError {
			t.Fatalf("get %v should fail: %v", args, err)
		}
	}
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_sessions", Arguments: map[string]any{"query": " "}})
	if err != nil || !res.IsError {
		t.Fatalf("empty search should fail: %v", err)
	}
	got := e.log.all()
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %+v", got)
	}
	for i, id := range []string{"ffff", "cccc0005"} {
		en := got[i]
		if en.Error != "not found" || inputOf(t, en)["id"] != id || len(en.SessionIDs) != 0 || en.Count != 0 {
			t.Errorf("unknown or Excluded get: %+v", en)
		}
	}
	if got[2].Error != "invalid input" {
		t.Errorf("search error = %q", got[2].Error)
	}
	b, _ := json.Marshal(got[0])
	if !strings.Contains(string(b), `"session_ids":[]`) {
		t.Errorf("session_ids should be an empty list, not null: %s", b)
	}
}

// TestAccessLogHoldsNoContent: the log file never quotes titles, message text, snippets or
// cwd paths; only what the client sent.
func TestAccessLogHoldsNoContent(t *testing.T) {
	e := setup(t)
	path := filepath.Join(t.TempDir(), "cache", "access.jsonl")
	log := NewAccessLog(path)
	e.cs = connect(t, e.load, log.Record)
	var l ListOutput
	e.call(t, "list_sessions", map[string]any{"limit": 100}, &l)
	var s SearchOutput
	e.call(t, "search_sessions", map[string]any{"query": "secret"}, &s)
	var g GetOutput
	e.call(t, "get_session", map[string]any{"id": "1111"}, &g)
	e.call(t, "get_session", map[string]any{"id": "2222"}, &g)

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 4 {
		t.Errorf("want 4 lines, got %d:\n%s", n, b)
	}
	for _, leak := range []string{
		"login-work", "Add login page", "hey, please add a login page", "Sure, starting with the form", "file contents",
		"general kenobi", "client secret stuff", "the secret is out", "/x/dev", "/x/work", "/tmp/repo", "needle", "permission denied",
	} {
		if bytes.Contains(b, []byte(leak)) {
			t.Errorf("log leaks %q:\n%s", leak, b)
		}
	}
	if !bytes.Contains(b, []byte(`"query":"secret"`)) {
		t.Errorf("the query the client sent belongs in the log:\n%s", b)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
}

func TestAccessLogRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	log := NewAccessLog(path)
	log.MaxSize = 300
	line := func(i int) Entry {
		return Entry{At: "2026-10-15T10:00:00Z", Tool: "list_sessions", Input: json.RawMessage(`{}`), SessionIDs: []string{fmt.Sprintf("id-%03d", i)}}
	}
	n := 0
	for ; ; n++ {
		log.Record(line(n))
		if _, err := os.Stat(path + ".1"); err == nil {
			break
		}
		if n > 100 {
			t.Fatal("never rotated")
		}
	}
	old, _ := os.ReadFile(path + ".1")
	cur, _ := os.ReadFile(path)
	if int64(len(old)) < log.MaxSize || strings.Count(string(cur), "\n") != 1 || !strings.Contains(string(cur), fmt.Sprintf("id-%03d", n)) {
		t.Errorf("rotation: old %d bytes, current %q", len(old), cur)
	}
	// The next rotation replaces the old .1.
	for i := 0; i < 2*n+2; i++ {
		log.Record(line(1000 + i))
	}
	old2, _ := os.ReadFile(path + ".1")
	if bytes.Contains(old2, []byte("id-000")) {
		t.Error("the second rotation should replace access.jsonl.1")
	}
	for _, p := range []string{path, path + ".1"} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v (%v)", p, fi.Mode().Perm(), err)
		}
	}
}

func TestAccessLogTightensMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	NewAccessLog(path).Record(Entry{Tool: "list_sessions"})
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestAccessLogConcurrentLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	log := NewAccessLog(path)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Record(Entry{Tool: "get_session", Input: json.RawMessage(fmt.Sprintf(`{"id":"%d"}`, i))})
		}()
	}
	wg.Wait()
	entries, err := ReadLog(path)
	if err != nil || len(entries) != 50 {
		t.Fatalf("want 50 whole lines, got %d (%v)", len(entries), err)
	}
}

// TestAccessLogWriteFailureNeverFailsTheCall: a log that can't be written is reported on
// stderr and the tool call still succeeds.
func TestAccessLogWriteFailureNeverFailsTheCall(t *testing.T) {
	e := setup(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	log := NewAccessLog(filepath.Join(blocker, "access.jsonl"))
	var stderr bytes.Buffer
	log.Stderr = &stderr
	e.cs = connect(t, e.load, log.Record)
	var l ListOutput
	e.call(t, "list_sessions", nil, &l)
	if l.Total == 0 {
		t.Error("the call should still return sessions")
	}
	if !strings.Contains(stderr.String(), "access log") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestReadLogMissing(t *testing.T) {
	entries, err := ReadLog(filepath.Join(t.TempDir(), "access.jsonl"))
	if err != nil || len(entries) != 0 {
		t.Errorf("missing log: %v %v", entries, err)
	}
}

func TestReadLogIncludesRotatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	if err := os.WriteFile(path+".1", []byte(`{"at":"2026-10-01T00:00:00Z","tool":"list_sessions"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json\n"+`{"at":"2026-10-02T00:00:00Z","tool":"get_session"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadLog(path)
	if err != nil || len(entries) != 2 || entries[0].Tool != "list_sessions" || entries[1].Tool != "get_session" {
		t.Errorf("entries = %+v (%v)", entries, err)
	}
}

func TestSummarize(t *testing.T) {
	at := func(d string) string { return "2026-10-" + d + "T10:00:00Z" }
	entries := []Entry{
		{At: at("01"), Tool: "get_session", Client: "old", SessionIDs: []string{"gone"}},
		{At: at("10"), Tool: "list_sessions", Client: "chatgpt"},
		{At: at("11"), Tool: "search_sessions", Client: "chatgpt", Input: json.RawMessage(`{"query":"auth bug"}`)},
		{At: at("12"), Tool: "search_sessions", Client: "chatgpt", Input: json.RawMessage(`{"query":"auth bug","offset":10}`)},
		{At: at("12"), Tool: "search_sessions", Client: "claude-ai", Input: json.RawMessage(`{"query":"deploy"}`)},
		{At: at("13"), Tool: "get_session", Client: "chatgpt", SessionIDs: []string{"s1"}},
		{At: at("13"), Tool: "get_session", Client: "chatgpt", SessionIDs: []string{"s1"}},
		{At: at("14"), Tool: "get_session", Client: "", SessionIDs: []string{"s2"}},
		{At: at("14"), Tool: "get_session", Client: "chatgpt", Input: json.RawMessage(`{"id":"nope"}`), SessionIDs: []string{}, Error: "not found"},
		{At: "garbage", Tool: "list_sessions"},
	}
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	st := Summarize(entries, since)
	if st.Calls != 8 || st.Errors != 1 {
		t.Errorf("calls=%d errors=%d", st.Calls, st.Errors)
	}
	if st.First.Format(time.RFC3339) != at("10") || st.Last.Format(time.RFC3339) != at("14") {
		t.Errorf("range %v to %v", st.First, st.Last)
	}
	if want := []Tally{{"get_session", 4}, {"search_sessions", 3}, {"list_sessions", 1}}; !reflect.DeepEqual(st.Tools, want) {
		t.Errorf("tools = %v", st.Tools)
	}
	if want := []Tally{{"chatgpt", 6}, {"(unknown)", 1}, {"claude-ai", 1}}; !reflect.DeepEqual(st.Clients, want) {
		t.Errorf("clients = %v", st.Clients)
	}
	if want := []Tally{{"s1", 2}, {"s2", 1}}; !reflect.DeepEqual(st.MostRead, want) {
		t.Errorf("most read = %v", st.MostRead)
	}
	if want := []Tally{{"auth bug", 2}, {"deploy", 1}}; !reflect.DeepEqual(st.TopQueries, want) {
		t.Errorf("queries = %v", st.TopQueries)
	}

	all := Summarize(entries, time.Time{})
	if all.Calls != 9 || len(all.MostRead) != 3 {
		t.Errorf("no window: calls=%d most read=%v", all.Calls, all.MostRead)
	}
	if empty := Summarize(nil, since); empty.Calls != 0 || empty.Tools == nil {
		t.Errorf("empty = %+v", empty)
	}
}

func TestSummarizeKeepsTopTen(t *testing.T) {
	var entries []Entry
	for i := 0; i < 15; i++ {
		for j := 0; j <= i; j++ {
			entries = append(entries, Entry{At: "2026-10-10T00:00:00Z", Tool: "get_session", SessionIDs: []string{fmt.Sprintf("s%02d", i)}})
		}
	}
	st := Summarize(entries, time.Time{})
	if len(st.MostRead) != 10 || st.MostRead[0] != (Tally{"s14", 15}) {
		t.Errorf("most read = %v", st.MostRead)
	}
}

// TestAccessLogClipsLongInputs: every string the client sends is cut to 500 bytes on a
// UTF-8 boundary, so one call can't write an unbounded line.
func TestAccessLogClipsLongInputs(t *testing.T) {
	e := setup(t)
	long := "a" + strings.Repeat("é", 2000) // odd start: the 500-byte cut lands mid-rune
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_sessions", Arguments: map[string]any{"query": long, "account": long, "project": long}})
	if err != nil || res.IsError {
		t.Fatalf("search: %v %v", err, res)
	}
	res, err = e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: map[string]any{"id": long}})
	if err != nil || !res.IsError {
		t.Fatalf("get should fail: %v", err)
	}
	got := e.log.all()
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	check := func(field string, v any) {
		s, _ := v.(string)
		if len(s) > 500 || !utf8.ValidString(s) || !strings.HasSuffix(s, "…") || !strings.HasPrefix(long, strings.TrimSuffix(s, "…")) {
			t.Errorf("%s not clipped: %d bytes, valid=%v, %q", field, len(s), utf8.ValidString(s), s[max(0, len(s)-10):])
		}
	}
	in := inputOf(t, got[0])
	for _, f := range []string{"query", "account", "project"} {
		check(f, in[f])
	}
	check("id", inputOf(t, got[1])["id"])
	if short := clipString("short"); short != "short" {
		t.Errorf("short strings stay: %q", short)
	}
}

// TestAccessLogErrorClasses: the error field is one of a few fixed words, never the
// error text, which can carry paths.
func TestAccessLogErrorClasses(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"failed", fmt.Errorf("open /Users/someone/.claude/projects/secret: permission denied"), "failed"},
		{"canceled", fmt.Errorf("scan: %w", context.Canceled), "canceled"},
		{"deadline", context.DeadlineExceeded, "canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorded{}
			cs := connect(t, func(context.Context) ([]*session.Session, []string, error) { return nil, nil, tc.err }, rec.record)
			if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_sessions"}); err != nil || !res.IsError {
				t.Fatalf("list should fail: %v", err)
			}
			got := rec.all()
			if len(got) != 1 || got[0].Error != tc.want {
				t.Errorf("entries = %+v, want error %q", got, tc.want)
			}
		})
	}
	e := setup(t)
	if res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: map[string]any{"id": " "}}); err != nil || !res.IsError {
		t.Fatalf("empty id should fail: %v", err)
	}
	if got := e.log.all(); len(got) != 1 || got[0].Error != "invalid input" {
		t.Errorf("empty id: %+v", got)
	}
}

// TestAccessLogRejectedCalls: calls the SDK turns away before the handler (bad argument
// types, unknown tools) are logged too, once each, as invalid input.
func TestAccessLogRejectedCalls(t *testing.T) {
	e := setup(t)
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: map[string]any{"id": 5}})
	if err != nil || !res.IsError {
		t.Fatalf("a bad argument type should fail: %v %v", err, res)
	}
	if _, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "no_such_tool"}); err == nil {
		t.Fatal("an unknown tool should fail")
	}
	var l ListOutput
	e.call(t, "list_sessions", nil, &l)
	got := e.log.all()
	if len(got) != 3 {
		t.Fatalf("want 3 entries (no double logging), got %+v", got)
	}
	for i, tool := range []string{"get_session", "no_such_tool"} {
		en := got[i]
		if en.Tool != tool || en.Error != "invalid input" || en.Client != "test-client" || en.SessionIDs == nil {
			t.Errorf("rejected %s: %+v", tool, en)
		}
	}
	if got[2].Tool != "list_sessions" || got[2].Error != "" {
		t.Errorf("handled call: %+v", got[2])
	}
}

func TestReadLogSkipsLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	huge := `{"at":"2026-10-01T00:00:00Z","tool":"search_sessions","input":{"query":"` + strings.Repeat("x", 3<<20) + `"}}`
	body := `{"at":"2026-10-01T00:00:00Z","tool":"list_sessions"}` + "\n" + huge + "\n" + `{"at":"2026-10-02T00:00:00Z","tool":"get_session"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadLog(path)
	if err != nil || len(entries) != 2 || entries[0].Tool != "list_sessions" || entries[1].Tool != "get_session" {
		t.Errorf("entries = %d %v", len(entries), err)
	}
}

// TestAccessLogRefusesSymlink: a symlink at the log path is never followed; the write
// fails on stderr and the target is untouched.
func TestAccessLogRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "access.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	log := NewAccessLog(path)
	var stderr bytes.Buffer
	log.Stderr = &stderr
	log.Record(Entry{Tool: "list_sessions"})
	if b, _ := os.ReadFile(target); string(b) != "keep\n" {
		t.Errorf("target written through the symlink: %q", b)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("target mode changed: %v", fi.Mode().Perm())
	}
	if !strings.Contains(stderr.String(), "access log") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// twoWriters runs n Records split across two AccessLogs on the same path, as two server
// processes would.
func twoWriters(t *testing.T, path string, maxSize int64, n int) {
	t.Helper()
	a, b := NewAccessLog(path), NewAccessLog(path)
	a.MaxSize, b.MaxSize = maxSize, maxSize
	var stderr syncBuffer
	a.Stderr, b.Stderr = &stderr, &stderr
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := a
			if i%2 == 1 {
				l = b
			}
			l.Record(Entry{At: "2026-10-15T10:00:00Z", Tool: "get_session", Input: json.RawMessage(fmt.Sprintf(`{"id":"%04d"}`, i))})
		}()
	}
	wg.Wait()
	if stderr.String() != "" {
		t.Fatalf("write failures: %s", stderr.String())
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestAccessLogTwoProcessesKeepEveryLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.jsonl")
	twoWriters(t, path, 10<<20, 400)
	entries, err := ReadLog(path)
	if err != nil || len(entries) != 400 {
		t.Fatalf("want 400 lines, got %d (%v)", len(entries), err)
	}
}

// TestAccessLogTwoProcessesRotateOnce: two writers that both see a full log must not both
// rotate, or the second rename replaces a full access.jsonl.1 with a nearly empty one.
func TestAccessLogTwoProcessesRotateOnce(t *testing.T) {
	const maxSize = 4 << 10
	for round := 0; round < 5; round++ {
		path := filepath.Join(t.TempDir(), "access.jsonl")
		twoWriters(t, path, maxSize, 600)
		fi, err := os.Stat(path + ".1")
		if err != nil {
			t.Fatalf("never rotated: %v", err)
		}
		if fi.Size() < maxSize {
			t.Fatalf("round %d: access.jsonl.1 is %d bytes, under MaxSize %d: a generation was lost", round, fi.Size(), maxSize)
		}
		cur, _ := os.ReadFile(path)
		if int64(len(cur)) >= maxSize+200 {
			t.Errorf("current file overgrew: %d bytes", len(cur))
		}
	}
}
