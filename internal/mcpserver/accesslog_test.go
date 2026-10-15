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

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	if got[2].Error != "query is empty" {
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
