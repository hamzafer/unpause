package statsweb

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

// zone is a fixed +02:00 so bucket boundaries are known regardless of the machine.
var zone = time.FixedZone("T", 2*3600)

// now is 2026-10-15 15:40 in zone.
var now = time.Date(2026, 10, 15, 15, 40, 0, 0, zone)

type ent struct {
	ago    time.Duration
	client string
	tool   string
	ids    []string
	dur    int64
	err    string
	input  string
}

func entries(es ...ent) []mcpserver.Entry {
	out := make([]mcpserver.Entry, 0, len(es))
	for _, e := range es {
		in := e.input
		if in == "" {
			in = `{}`
		}
		ids := e.ids
		if ids == nil {
			ids = []string{}
		}
		out = append(out, mcpserver.Entry{
			At: now.Add(-e.ago).UTC().Format(time.RFC3339), Client: e.client, Tool: e.tool,
			Input: json.RawMessage(in), SessionIDs: ids, Count: len(ids), Total: len(ids), DurationMS: e.dur, Error: e.err,
		})
	}
	return out
}

func noLookup(string) (Info, bool) { return Info{}, false }

func build(t *testing.T, es []mcpserver.Entry, rng string) Stats {
	t.Helper()
	st, err := BuildStats(es, rng, now, zone, noLookup)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRanges(t *testing.T) {
	for _, r := range []string{"24h", "7d", "30d", "all"} {
		st, err := BuildStats(nil, r, now, zone, noLookup)
		if err != nil || st.Range != r {
			t.Errorf("range %q: %v %q", r, err, st.Range)
		}
	}
	for _, r := range []string{"", "1h", "ALL", "7D", "365d", "all "} {
		if _, err := BuildStats(nil, r, now, zone, noLookup); err == nil {
			t.Errorf("range %q should be refused", r)
		}
	}
}

func TestEmpty(t *testing.T) {
	st := build(t, nil, "7d")
	if !st.Empty || st.KPIs.Calls != 0 || st.KPIs.P50MS != nil || st.KPIs.Prev == nil {
		t.Errorf("empty: %+v", st)
	}
	if st.GeneratedAt != "2026-10-15T13:40:00Z" {
		t.Errorf("generated_at %q", st.GeneratedAt)
	}
	for _, p := range st.Series.Points {
		if p.Covered {
			t.Errorf("no log, nothing is covered: %+v", p)
		}
	}
	b, _ := json.Marshal(st)
	for _, want := range []string{`"clients":[]`, `"most_read":[]`, `"top_queries":[]`, `"tools":[]`, `"recent":[]`, `"p50_ms":null`, `"by_client":{}`, `"empty":true`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	// A log with only unrelated old calls is not empty, just quiet in range.
	st = build(t, entries(ent{ago: 60 * 24 * time.Hour, client: "x", tool: "list_sessions"}), "24h")
	if st.Empty || st.KPIs.Calls != 0 {
		t.Errorf("old log: empty=%v calls=%d", st.Empty, st.KPIs.Calls)
	}
}

func TestKPIsAndPrev(t *testing.T) {
	h := time.Hour
	d := 24 * h
	es := entries(
		// prev window of 7d: 7 to 14 days ago
		ent{ago: 10 * d, client: "a", tool: "get_session", ids: []string{"s1"}, dur: 90},
		ent{ago: 9 * d, client: "a", tool: "search_sessions", dur: 50, err: "failed"},
		// this window
		ent{ago: 3 * d, client: "a", tool: "get_session", ids: []string{"s1"}, dur: 10},
		ent{ago: 2 * d, client: "b", tool: "get_session", ids: []string{"s2"}, dur: 30},
		ent{ago: 1 * d, client: "b", tool: "get_session", ids: []string{"s1"}, dur: 20},
		ent{ago: 2 * h, client: "b", tool: "get_session", ids: []string{"s9"}, dur: 1000, err: "not found"},
		ent{ago: 1 * h, client: "c", tool: "search_sessions", ids: []string{"s3"}, dur: 40},
		// way back, in no window but all
		ent{ago: 40 * d, client: "z", tool: "list_sessions", dur: 5},
	)
	st := build(t, es, "7d")
	k := st.KPIs
	if k.Calls != 5 || k.SessionsRead != 2 || k.Clients != 3 || k.Errors != 1 {
		t.Errorf("kpis %+v", k.Window)
	}
	// successful durations 10 20 30 40: lower median is 20; the failed 1000 doesn't count
	if k.P50MS == nil || *k.P50MS != 20 {
		t.Errorf("p50 %v", k.P50MS)
	}
	p := k.Prev
	if p == nil || p.Calls != 2 || p.SessionsRead != 1 || p.Clients != 1 || p.Errors != 1 || p.P50MS == nil || *p.P50MS != 90 {
		t.Errorf("prev %+v", p)
	}

	all := build(t, es, "all")
	if all.KPIs.Prev != nil || all.KPIs.Calls != 8 {
		t.Errorf("all: prev %v calls %d", all.KPIs.Prev, all.KPIs.Calls)
	}
	b, _ := json.Marshal(all)
	if !strings.Contains(string(b), `"prev":null`) {
		t.Errorf("all needs prev null: %s", b)
	}

	// p50 is null when every call in the window failed.
	st = build(t, entries(ent{ago: h, client: "a", tool: "get_session", err: "failed", dur: 3}), "24h")
	if st.KPIs.P50MS != nil || st.KPIs.Errors != 1 {
		t.Errorf("all failed: %+v", st.KPIs.Window)
	}
	// an odd count takes the middle
	st = build(t, entries(
		ent{ago: h, client: "a", tool: "x", dur: 7}, ent{ago: h, client: "a", tool: "x", dur: 3}, ent{ago: h, client: "a", tool: "x", dur: 100},
	), "24h")
	if st.KPIs.P50MS == nil || *st.KPIs.P50MS != 7 {
		t.Errorf("odd p50 %v", st.KPIs.P50MS)
	}
}

func TestHourlyBuckets(t *testing.T) {
	// first call ever is at 03:10 today; now is 15:40
	es := entries(
		ent{ago: 12*time.Hour + 30*time.Minute, client: "a", tool: "x"},
		ent{ago: 10 * time.Minute, client: "a", tool: "x"},
		ent{ago: 5 * time.Minute, client: "b", tool: "x"},
	)
	st := build(t, es, "24h")
	if st.Series.Bucket != "hour" {
		t.Errorf("bucket %q", st.Series.Bucket)
	}
	ps := st.Series.Points
	// The hour holding the window start (yesterday 15:00) through this hour: 25 points.
	if len(ps) != 25 {
		t.Fatalf("%d points", len(ps))
	}
	if ps[0].T != "2026-10-14T13:00:00Z" || ps[24].T != "2026-10-15T13:00:00Z" {
		t.Errorf("first %s last %s", ps[0].T, ps[24].T)
	}
	sum := 0
	for i, p := range ps {
		at, _ := time.Parse(time.RFC3339, p.T)
		local := at.In(zone)
		wantCovered := !local.Before(time.Date(2026, 10, 15, 3, 0, 0, 0, zone))
		if p.Covered != wantCovered {
			t.Errorf("point %d %s covered=%v", i, local, p.Covered)
		}
		for _, n := range p.ByClient {
			sum += n
		}
	}
	if sum != st.KPIs.Calls {
		t.Errorf("series sums to %d, calls %d", sum, st.KPIs.Calls)
	}
	last := ps[24].ByClient
	if last["a"] != 1 || last["b"] != 1 {
		t.Errorf("last hour %v", last)
	}
	if ps[12].ByClient["a"] != 1 { // 03:00 local
		t.Errorf("03:00 bucket %v", ps[12].ByClient)
	}
	if !ps[13].Covered || len(ps[13].ByClient) != 0 {
		t.Errorf("a quiet hour after the first call is a real zero: %+v", ps[13])
	}
}

func TestDailyBuckets(t *testing.T) {
	// 00:30 local on Oct 13 is 22:30 UTC on Oct 12: it belongs to the local day Oct 13.
	at := time.Date(2026, 10, 13, 0, 30, 0, 0, zone)
	es := entries(ent{ago: now.Sub(at), client: "a", tool: "x"})
	st := build(t, es, "7d")
	ps := st.Series.Points
	if st.Series.Bucket != "day" || len(ps) != 8 {
		t.Fatalf("bucket %q, %d points", st.Series.Bucket, len(ps))
	}
	if ps[0].T != "2026-10-07T22:00:00Z" || ps[7].T != "2026-10-14T22:00:00Z" {
		t.Errorf("first %s last %s", ps[0].T, ps[7].T)
	}
	for i, p := range ps {
		if p.Covered != (i >= 5) {
			t.Errorf("day %d covered=%v", i, p.Covered)
		}
	}
	if ps[5].ByClient["a"] != 1 {
		t.Errorf("Oct 13 bucket %+v", ps[5])
	}

	if n := len(build(t, es, "30d").Series.Points); n != 31 {
		t.Errorf("30d: %d points", n)
	}
	all := build(t, es, "all")
	if all.Series.Bucket != "day" || len(all.Series.Points) != 3 || all.Series.Points[0].T != "2026-10-12T22:00:00Z" {
		t.Errorf("all: %+v", all.Series)
	}
	for _, p := range all.Series.Points {
		if !p.Covered {
			t.Errorf("all starts at the first call, so every day is covered: %+v", p)
		}
	}
	if empty := build(t, nil, "all"); len(empty.Series.Points) != 1 || empty.Series.Points[0].Covered {
		t.Errorf("all with no log: %+v", empty.Series)
	}
}

func TestDailyBucketsAcrossDST(t *testing.T) {
	oslo, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		t.Skip("no tz data:", err)
	}
	// Summer time ends on 2026-10-25 in Oslo, so one day in the window is 25 hours.
	n := time.Date(2026, 11, 1, 12, 0, 0, 0, oslo)
	st, err := BuildStats(nil, "30d", n, oslo, noLookup)
	if err != nil {
		t.Fatal(err)
	}
	prev := time.Time{}
	for _, p := range st.Series.Points {
		at, _ := time.Parse(time.RFC3339, p.T)
		l := at.In(oslo)
		if l.Hour() != 0 || l.Minute() != 0 {
			t.Errorf("point %s is not local midnight (%s)", p.T, l)
		}
		if !prev.IsZero() && l.YearDay()-prev.YearDay() != 1 {
			t.Errorf("gap between %s and %s", prev, l)
		}
		prev = l
	}
	if len(st.Series.Points) != 31 {
		t.Errorf("%d points", len(st.Series.Points))
	}
}

func TestSlotsStableAcrossRanges(t *testing.T) {
	d := 24 * time.Hour
	es := entries(
		ent{ago: 20 * d, client: "c0", tool: "x"},
		ent{ago: 19 * d, client: "c1", tool: "x"},
		ent{ago: 18 * d, client: "c0", tool: "x"},
		ent{ago: 17 * d, client: "c2", tool: "x"},
		ent{ago: 16 * d, client: "c3", tool: "x"},
		ent{ago: 15 * d, client: "c4", tool: "x"},
		ent{ago: 14 * d, client: "c5", tool: "x"},
		ent{ago: time.Hour, client: "c5", tool: "x"},
		ent{ago: time.Hour, client: "c5", tool: "x"},
		ent{ago: 2 * time.Hour, client: "c1", tool: "x"},
	)
	want := map[string]int{"c0": 0, "c1": 1, "c2": 2, "c3": 3, "c4": 4, "c5": -1}
	for _, r := range []string{"24h", "30d", "all"} {
		st := build(t, es, r)
		for _, c := range st.Clients {
			if c.Slot != want[c.ID] {
				t.Errorf("%s: %s slot %d, want %d", r, c.ID, c.Slot, want[c.ID])
			}
		}
	}
	st := build(t, es, "24h")
	if len(st.Clients) != 2 || st.Clients[0].ID != "c5" || st.Clients[0].Calls != 2 || st.Clients[1].ID != "c1" {
		t.Errorf("24h clients by calls: %+v", st.Clients)
	}
	if st.Clients[0].LastSeen != now.Add(-time.Hour).UTC().Format(time.RFC3339) {
		t.Errorf("last_seen %q", st.Clients[0].LastSeen)
	}
}

func TestFriendlyName(t *testing.T) {
	for in, want := range map[string]string{
		"local-agent-mode-unpause": "Claude Cowork",
		"local-agent-mode-":        "Claude Cowork",
		"claude-ai":                "Claude Desktop",
		"openai-mcp":               "ChatGPT",
		"ChatGPT-Connector":        "ChatGPT",
		"my-chatgpt":               "ChatGPT",
		"codex-mcp-client":         "Codex",
		"codex":                    "Codex",
		"Codex":                    "Codex",
		"mycodex":                  "mycodex",
		"claude-ai-beta":           "claude-ai-beta",
		"(unknown)":                "(unknown)",
		"cursor":                   "cursor",
	} {
		if got := FriendlyName(in); got != want {
			t.Errorf("FriendlyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMostRead(t *testing.T) {
	var es []ent
	for i := range 12 {
		for range 12 - i { // s00 read 12 times, s11 once
			es = append(es, ent{ago: time.Duration(i+1) * time.Minute, client: "a", tool: "get_session", ids: []string{fmt.Sprintf("s%02d-0000-0000", i)}})
		}
	}
	// failed reads and search hits don't count as reads
	es = append(es,
		ent{ago: time.Second, client: "a", tool: "get_session", ids: []string{"s11-0000-0000"}, err: "failed"},
		ent{ago: time.Second, client: "a", tool: "search_sessions", ids: []string{"s11-0000-0000"}},
	)
	calls := 0
	lookup := func(id string) (Info, bool) {
		calls++
		if id == "s00-0000-0000" {
			return Info{Title: "Login work", Account: "work", Project: "repo"}, true
		}
		return Info{}, false
	}
	st, err := BuildStats(entries(es...), "24h", now, zone, lookup)
	if err != nil {
		t.Fatal(err)
	}
	mr := st.MostRead
	if len(mr) != 10 {
		t.Fatalf("%d most read", len(mr))
	}
	if calls > 10 {
		t.Errorf("lookup called %d times, want only for the top 10", calls)
	}
	top := mr[0]
	if top.ID != "s00-0000-0000" || top.Reads != 12 || top.Title != "Login work" || top.Account != "work" || top.Project != "repo" {
		t.Errorf("top %+v", top)
	}
	if top.LastRead != now.Add(-time.Minute).UTC().Format(time.RFC3339) {
		t.Errorf("last_read %q", top.LastRead)
	}
	if mr[1].Title != "s01-0000" || mr[1].Account != "" || mr[1].Project != "" {
		t.Errorf("unindexed session should show its short id: %+v", mr[1])
	}

	// no reads: the index is never touched
	calls = 0
	if _, err := BuildStats(entries(ent{ago: time.Minute, client: "a", tool: "list_sessions"}), "24h", now, zone, lookup); err != nil || calls != 0 {
		t.Errorf("lookup called %d times with nothing read", calls)
	}
}

func TestQueriesToolsRecent(t *testing.T) {
	var es []ent
	for i := range 60 {
		es = append(es, ent{ago: time.Duration(60-i) * time.Minute, client: "a", tool: "list_sessions", input: fmt.Sprintf(`{"offset":%d}`, i)})
	}
	es = append(es,
		ent{ago: 30 * time.Second, client: "b", tool: "search_sessions", input: `{"query":"  auth bug "}`, ids: []string{"s1"}},
		ent{ago: 20 * time.Second, client: "b", tool: "search_sessions", input: `{"query":"auth bug"}`},
		ent{ago: 10 * time.Second, client: "openai-mcp", tool: "search_sessions", input: `{"query":"fxtwitter"}`, err: "invalid input"},
	)
	st := build(t, entries(es...), "24h")
	if len(st.TopQueries) != 2 || st.TopQueries[0] != (Query{Query: "auth bug", Count: 2}) || st.TopQueries[1].Query != "fxtwitter" {
		t.Errorf("top_queries %+v", st.TopQueries)
	}
	if len(st.Tools) != 2 || st.Tools[0] != (Tool{Name: "list_sessions", Count: 60}) || st.Tools[1] != (Tool{Name: "search_sessions", Count: 3}) {
		t.Errorf("tools %+v", st.Tools)
	}
	r := st.Recent
	if len(r) != 50 {
		t.Fatalf("%d recent", len(r))
	}
	first := r[0]
	if first.Client != "openai-mcp" || first.ClientName != "ChatGPT" || first.Error != "invalid input" || string(first.Input) != `{"query":"fxtwitter"}` {
		t.Errorf("newest %+v", first)
	}
	if first.SessionIDs == nil {
		t.Error("session_ids must be [] not null")
	}
	if r[2].SessionIDs[0] != "s1" || r[2].Count != 1 || r[2].Total != 1 {
		t.Errorf("third %+v", r[2])
	}
	if string(r[49].Input) != `{"offset":13}` {
		t.Errorf("oldest kept %s", r[49].Input)
	}
	b, _ := json.Marshal(first)
	for _, k := range []string{`"at":`, `"client":`, `"client_name":`, `"tool":`, `"input":{`, `"session_ids":[]`, `"count":`, `"total":`, `"duration_ms":`, `"error":`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("recent row missing %s: %s", k, b)
		}
	}
}

func TestUnknownClientAndBadTimes(t *testing.T) {
	es := entries(ent{ago: time.Minute, client: "", tool: "x"})
	es = append(es, mcpserver.Entry{At: "garbage", Client: "y", Tool: "x"})
	st := build(t, es, "all")
	if st.KPIs.Calls != 1 || len(st.Clients) != 1 || st.Clients[0].ID != "(unknown)" || st.Clients[0].Slot != 0 {
		t.Errorf("%+v %+v", st.KPIs.Window, st.Clients)
	}
}

func TestBuildSession(t *testing.T) {
	es := entries(
		ent{ago: 3 * time.Hour, client: "claude-ai", tool: "search_sessions", ids: []string{"s1-aaaa-bbbb", "s2"}},
		ent{ago: 2 * time.Hour, client: "codex", tool: "get_session", ids: []string{"s1-aaaa-bbbb"}},
		ent{ago: time.Hour, client: "codex", tool: "get_session", ids: []string{"s2"}},
	)
	lookup := func(id string) (Info, bool) {
		if id == "s1-aaaa-bbbb" {
			return Info{Title: "T", Account: "work", Project: "p"}, true
		}
		return Info{}, false
	}
	s, ok := BuildSession(es, "s1-aaaa-bbbb", lookup)
	if !ok || s.ID != "s1-aaaa-bbbb" || s.Title != "T" || s.Account != "work" || s.Project != "p" || len(s.Calls) != 2 {
		t.Fatalf("%v %+v", ok, s)
	}
	if s.Calls[0].Tool != "get_session" || s.Calls[0].ClientName != "Codex" || s.Calls[1].ClientName != "Claude Desktop" || s.Calls[1].Count != 2 {
		t.Errorf("calls newest first: %+v", s.Calls)
	}
	s, ok = BuildSession(es, "s2", lookup)
	if !ok || s.Title != "s2" {
		t.Errorf("unindexed: %+v", s)
	}
	if _, ok := BuildSession(es, "nope", lookup); ok {
		t.Error("an id no call returned is unknown")
	}
	if _, ok := BuildSession(es, "s1", lookup); ok {
		t.Error("a prefix is not an id")
	}
}
