package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/session"
)

// fixture is one transcript the loader will serve.
type fixture struct {
	id, account, cwd string
	at               time.Time
	prompts          []string
	live             string
}

func writeTranscript(t *testing.T, dir string, f fixture) string {
	t.Helper()
	var b strings.Builder
	for i, p := range f.prompts {
		ts := f.at.Add(time.Duration(i-len(f.prompts)) * time.Minute).Format(time.RFC3339Nano)
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content, _ := json.Marshal(p)
		if role == "assistant" {
			content, _ = json.Marshal([]map[string]string{{"type": "text", "text": p}})
		}
		fmt.Fprintf(&b, `{"type":%q,"message":{"role":%q,"content":%s},"timestamp":%q,"cwd":%q,"sessionId":%q,"gitBranch":"main"}`+"\n",
			role, role, content, ts, f.cwd, f.id)
	}
	p := filepath.Join(dir, f.id+".jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type env struct {
	dir     string
	exclude []string
	cs      *mcp.ClientSession
	load    Loader
}

// setup writes the fixture root and connects an in-memory client to a server over it.
func setup(t *testing.T, exclude ...string) *env {
	t.Helper()
	dir := t.TempDir()
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	fixtures := []fixture{
		{id: "aaaa0001-0000-0000-0000-000000000000", account: "personal", cwd: "/x/dev/unpause", at: base, prompts: []string{"hello there", "general kenobi"}, live: "busy"},
		{id: "aaaa0002-0000-0000-0000-000000000000", account: "personal", cwd: "/x/dev/other", at: base.Add(-1 * time.Hour), prompts: []string{"fix the Widget", "widget fixed"}},
		{id: "bbbb0003-0000-0000-0000-000000000000", account: "work", cwd: "/x/work/client", at: base.Add(-2 * time.Hour), prompts: []string{"client secret stuff", "ok"}},
		{id: "bbbb0004-0000-0000-0000-000000000000", account: "work", cwd: "/x/work/client/sub/deeper", at: base.Add(-3 * time.Hour), prompts: []string{"deeper secret stuff", "ok"}},
		{id: "cccc0005-0000-0000-0000-000000000000", account: "work", cwd: "/x/dev/secret-repo", at: base.Add(-4 * time.Hour), prompts: []string{"repo secret", "ok"}},
	}
	for i := 0; i < 5; i++ {
		fixtures = append(fixtures, fixture{
			id: fmt.Sprintf("dddd%04d-0000-0000-0000-000000000000", i), account: "personal", cwd: "/x/dev/pager",
			at: base.Add(-time.Duration(10+i) * time.Hour), prompts: []string{fmt.Sprintf("page prompt %d", i), "reply"},
		})
	}
	paths := map[string]fixture{}
	for _, f := range fixtures {
		paths[writeTranscript(t, dir, f)] = f
	}
	// The richer fixtures from the claude package: 1111... (basic) and 2222... (blocks).
	copyFile(t, "../provider/claude/testdata/basic.jsonl", filepath.Join(dir, "basic.jsonl"))
	copyFile(t, "../provider/claude/testdata/blocks.jsonl", filepath.Join(dir, "blocks.jsonl"))

	load := func(ctx context.Context) ([]*session.Session, []string, error) {
		var out []*session.Session
		names, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		for _, p := range names {
			s, err := claude.ScanFile(p)
			if err != nil {
				return nil, nil, err
			}
			s.Account = "personal"
			if f, ok := paths[p]; ok {
				s.Account = f.account
				if f.live != "" {
					s.Live = &session.Live{PID: 1, Status: f.live}
				}
			}
			out = append(out, s)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
		return out, exclude, nil
	}

	return &env{dir: dir, exclude: exclude, cs: connect(t, load), load: load}
}

// connect starts a server over load and returns an in-memory client session to it.
func connect(t *testing.T, load Loader) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	srv := New("test-version", load)
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and decodes its structured output into out. It fails on tool errors.
func (e *env) call(t *testing.T, name string, args map[string]any, out any) {
	t.Helper()
	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: tool error: %s", name, textOf(res))
	}
	b, _ := json.Marshal(res.StructuredContent)
	reflect.ValueOf(out).Elem().SetZero()
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: decode %s: %v", name, b, err)
	}
}

func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func ids(ss []Summary) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID[:8])
	}
	return out
}

func TestServerInfoAndTools(t *testing.T) {
	e := setup(t)
	info := e.cs.InitializeResult().ServerInfo
	if info.Name != "unpause" || info.Version != "test-version" {
		t.Errorf("server info = %+v", info)
	}
	res, err := e.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
	for _, name := range []string{"list_sessions", "search_sessions", "get_session"} {
		if !got[name] {
			t.Errorf("missing tool %s", name)
		}
	}
	if len(res.Tools) != 3 {
		t.Errorf("want exactly 3 tools, got %d", len(res.Tools))
	}
}

func TestListSessionsPaging(t *testing.T) {
	e := setup(t)
	var out ListOutput
	e.call(t, "list_sessions", map[string]any{"limit": 3}, &out)
	if out.Total != 12 || out.Offset != 0 || len(out.Sessions) != 3 || out.NextOffset == nil || *out.NextOffset != 3 {
		t.Fatalf("page 1: total=%d offset=%d next=%v ids=%v", out.Total, out.Offset, out.NextOffset, ids(out.Sessions))
	}
	if got := ids(out.Sessions); strings.Join(got, ",") != "aaaa0001,aaaa0002,bbbb0003" {
		t.Errorf("newest first: %v", got)
	}
	first := out.Sessions[0]
	if first.Title != "hello there" || first.Account != "personal" || first.Project != "unpause" || first.CWD != "/x/dev/unpause" ||
		first.Branch != "main" || first.Live != "busy" || first.Messages != 2 || first.LastActive != "2026-09-10T11:59:00Z" || first.Created != "2026-09-10T11:58:00Z" {
		t.Errorf("summary = %+v", first)
	}
	if out.Sessions[1].Live != "" {
		t.Errorf("idle-less session must omit live: %+v", out.Sessions[1])
	}

	var last ListOutput
	e.call(t, "list_sessions", map[string]any{"limit": 5, "offset": 10}, &last)
	if len(last.Sessions) != 2 || last.NextOffset != nil || last.Offset != 10 {
		t.Errorf("last page: %+v", last)
	}

	var def ListOutput
	e.call(t, "list_sessions", map[string]any{}, &def)
	if len(def.Sessions) != 12 {
		t.Errorf("default limit 20 should return all 12, got %d", len(def.Sessions))
	}
	var big ListOutput
	e.call(t, "list_sessions", map[string]any{"limit": 1000}, &big)
	if len(big.Sessions) != 12 {
		t.Errorf("limit over max is clamped, not refused: got %d", len(big.Sessions))
	}
}

func TestListSessionsFilters(t *testing.T) {
	e := setup(t)
	var out ListOutput
	e.call(t, "list_sessions", map[string]any{"account": "work"}, &out)
	if strings.Join(ids(out.Sessions), ",") != "bbbb0003,bbbb0004,cccc0005" || out.Total != 3 {
		t.Errorf("account=work: %v", ids(out.Sessions))
	}
	e.call(t, "list_sessions", map[string]any{"project": "PAGER"}, &out)
	if out.Total != 5 {
		t.Errorf("project=PAGER (case-insensitive cwd substring): total %d", out.Total)
	}
	e.call(t, "list_sessions", map[string]any{"live": true}, &out)
	if strings.Join(ids(out.Sessions), ",") != "aaaa0001" {
		t.Errorf("live=true: %v", ids(out.Sessions))
	}
	e.call(t, "list_sessions", map[string]any{"live": false}, &out)
	if out.Total != 11 {
		t.Errorf("live=false: total %d", out.Total)
	}
}

func TestExclude(t *testing.T) {
	e := setup(t, "/x/work/*", "*/secret-repo")
	var out ListOutput
	e.call(t, "list_sessions", map[string]any{"limit": 100}, &out)
	for _, s := range out.Sessions {
		if strings.HasPrefix(s.CWD, "/x/work/") || strings.HasSuffix(s.CWD, "secret-repo") {
			t.Errorf("excluded session listed: %s %s", s.ID, s.CWD)
		}
	}
	if out.Total != 9 {
		t.Errorf("total = %d, want 9 (12 minus client, client/sub/deeper, secret-repo)", out.Total)
	}

	var sr SearchOutput
	e.call(t, "search_sessions", map[string]any{"query": "secret"}, &sr)
	// Only basic.jsonl (cwd /tmp/repo, thinking "secret") is left.
	if sr.Total != 1 || sr.Results[0].ID[:8] != "11111111" {
		t.Errorf("search found excluded sessions: %v", ids(summaries(sr.Results)))
	}

	res, err := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: map[string]any{"id": "bbbb0004"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(textOf(res), "not found") {
		t.Errorf("get_session on an excluded session must say not found, got %s", textOf(res))
	}
}

func TestExcluded(t *testing.T) {
	cases := []struct {
		pattern, cwd string
		want         bool
	}{
		{"/x/work/*", "/x/work/client", true},
		{"/x/work/*", "/x/work/client/sub/deeper", true},
		{"/x/work/*", "/x/work", false},
		{"/x/work/*", "/x/workshop/a", false},
		{"/x/work", "/x/work/a/b", true},
		{"*/secret-repo", "/x/dev/secret-repo", true},
		{"*/secret-repo", "/x/dev/secret-repo/pkg", true},
		{"*/secret-repo", "/x/dev/secret-repo-2", false},
		{"secret-repo", "/x/dev/secret-repo", true},
		// No folder at all: hidden whenever any exclude is set (fail closed).
		{"/x/dev/*", "", true},
	}
	for _, c := range cases {
		if got := newExcluder([]string{c.pattern}).hides(&session.Session{CWD: c.cwd}); got != c.want {
			t.Errorf("hides(%q, %q) = %v, want %v", c.cwd, c.pattern, got, c.want)
		}
	}
	if newExcluder(nil).hides(&session.Session{}) {
		t.Error("with no exclude set, a Session without a folder is visible")
	}
}

func TestExcludedChecksEveryFolder(t *testing.T) {
	x := newExcluder([]string{"/x/work/*", "*/Downloads/foo"})
	// Worked in a work folder first, then moved on: still hidden.
	if !x.hides(&session.Session{CWD: "/x/dev/ok", Dirs: []string{"/x/work/client", "/x/dev/ok"}}) {
		t.Error("a Session is Excluded when any folder it worked in matches")
	}
	// A Cowork Session: VM cwd, plus the host folder the user gave it.
	if !x.hides(&session.Session{CWD: "/sessions/edison", Dirs: []string{"/sessions/edison", "/Users/me/Downloads/foo"}, Cowork: true}) {
		t.Error("a Cowork Session is Excluded by the folders it was given")
	}
	if x.hides(&session.Session{CWD: "/x/dev/ok", Dirs: []string{"/x/dev/ok", "/x/dev/other"}}) {
		t.Error("no folder matches, so the Session is visible")
	}
	// Dirs alone, CWD empty.
	if !x.hides(&session.Session{Dirs: []string{"/x/work/a"}}) {
		t.Error("Dirs count without a CWD")
	}
}

func TestExcludedFoldsCaseOnMac(t *testing.T) {
	old := foldCase
	t.Cleanup(func() { foldCase = old })

	foldCase = true
	if !newExcluder([]string{"/X/Work/*"}).hides(&session.Session{CWD: "/x/WORK/client"}) {
		t.Error("case-insensitive match expected when folding")
	}
	foldCase = false
	if newExcluder([]string{"/X/Work/*"}).hides(&session.Session{CWD: "/x/WORK/client"}) {
		t.Error("case-sensitive match expected when not folding")
	}
}

func TestExcludedResolvesSymlinks(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real", "secret")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "real"), link); err != nil {
		t.Fatal(err)
	}
	x := newExcluder([]string{filepath.Join(dir, "real", "*")})
	if !x.hides(&session.Session{CWD: filepath.Join(link, "secret")}) {
		t.Error("a folder reached through a symlink must match the pattern on its real path")
	}
	// The raw path still counts too.
	if !newExcluder([]string{filepath.Join(link, "*")}).hides(&session.Session{CWD: filepath.Join(link, "secret")}) {
		t.Error("the raw path must match as well")
	}
	// A symlink inside the pattern counts too.
	if !newExcluder([]string{filepath.Join(link, "secret")}).hides(&session.Session{CWD: real}) {
		t.Error("a pattern written through a symlink must match the real folder")
	}
	if !newExcluder([]string{filepath.Join(link, "*")}).hides(&session.Session{CWD: real}) {
		t.Error("a glob written through a symlink must match the real folder")
	}
}

func TestSearchCountsDesktopTitle(t *testing.T) {
	dir := t.TempDir()
	p := writeTranscript(t, dir, fixture{id: "eeee0001-0000-0000-0000-000000000000", cwd: "/sessions/vm", at: time.Now(), prompts: []string{"make me a chart", "done"}})
	cs := connect(t, func(context.Context) ([]*session.Session, []string, error) {
		s, err := claude.ScanFile(p)
		if err != nil {
			return nil, nil, err
		}
		s.Account, s.Cowork, s.DesktopTitle = "cowork", true, "Quarterly dashboard"
		return []*session.Session{s}, nil, nil
	})
	e := &env{dir: dir, cs: cs}
	var out SearchOutput
	e.call(t, "search_sessions", map[string]any{"query": "quarterly"}, &out)
	if out.Total != 1 || out.Results[0].Hits != 1 {
		t.Errorf("a Desktop title must be searched: %+v", out)
	}
}

func TestPagingAtMaxOffset(t *testing.T) {
	e := setup(t)
	// The SDK decodes arguments through float64, so math.MaxInt itself can't come over
	// the wire; call the handlers directly with it.
	h := &handlers{load: e.load}
	ctx := context.Background()
	_, lo, err := h.list(ctx, nil, ListInput{Offset: math.MaxInt, Limit: listMax})
	if err != nil || len(lo.Sessions) != 0 || lo.NextOffset != nil || lo.Total != 12 {
		t.Errorf("list at MaxInt: %v %+v", err, lo)
	}
	_, so, err := h.search(ctx, nil, SearchInput{Query: "secret", Offset: math.MaxInt, Limit: searchMax})
	if err != nil || len(so.Results) != 0 || so.NextOffset != nil || so.Total != 4 {
		t.Errorf("search at MaxInt: %v %+v", err, so)
	}
	_, g, err := h.get(ctx, nil, GetInput{ID: "22222222", Offset: math.MaxInt, Limit: getMax})
	if err != nil || len(g.Messages) != 0 || g.NextOffset != nil || g.Total != 5 {
		t.Errorf("get at MaxInt: %v total=%d next=%v msgs=%d", err, g.Total, g.NextOffset, len(g.Messages))
	}

	// Over the wire: the largest integer offset JSON carries without overflowing int.
	const big = 1 << 62
	var wl ListOutput
	e.call(t, "list_sessions", map[string]any{"offset": big}, &wl)
	if len(wl.Sessions) != 0 || wl.NextOffset != nil {
		t.Errorf("list at 2^62: %+v", wl)
	}
	var wg GetOutput
	e.call(t, "get_session", map[string]any{"id": "22222222", "offset": big}, &wg)
	if len(wg.Messages) != 0 || wg.NextOffset != nil {
		t.Errorf("get at 2^62: %+v", wg)
	}
}

func TestSearchSessions(t *testing.T) {
	e := setup(t)
	var out SearchOutput
	e.call(t, "search_sessions", map[string]any{"query": "WIDGET"}, &out)
	if out.Total != 1 || out.Results[0].ID[:8] != "aaaa0002" {
		t.Fatalf("WIDGET: %+v", out)
	}
	r := out.Results[0]
	// Prompt (1) + reply "widget fixed" (1). The title is the first prompt, so it is not counted twice.
	if r.Hits != 2 || len(r.Snippets) != 2 || r.Snippets[0] != "fix the Widget" {
		t.Errorf("hits=%d snippets=%q", r.Hits, r.Snippets)
	}

	e.call(t, "search_sessions", map[string]any{"query": "login"}, &out)
	// basic.jsonl: the Name "login-work" (1), prompt "login page" (1), reply "login.tsx" (1).
	if out.Total != 1 || out.Results[0].Hits != 3 {
		t.Errorf("login: %+v", out)
	}

	e.call(t, "search_sessions", map[string]any{"query": "needle"}, &out)
	if out.Total != 1 || out.Results[0].ID[:8] != "22222222" || out.Results[0].Hits != 4 {
		t.Errorf("needle across tool input and results: %+v", out)
	}

	e.call(t, "search_sessions", map[string]any{"query": "secret"}, &out)
	// bbbb0003, bbbb0004, cccc0005 in prompts, and 1111 (basic) has thinking "secret".
	if strings.Join(ids(summaries(out.Results)), ",") != "bbbb0003,bbbb0004,cccc0005,11111111" {
		t.Errorf("secret, newest first: %v", ids(summaries(out.Results)))
	}
	e.call(t, "search_sessions", map[string]any{"query": "secret", "account": "work", "limit": 2}, &out)
	if out.Total != 3 || len(out.Results) != 2 || out.NextOffset == nil || *out.NextOffset != 2 {
		t.Errorf("paged and filtered: total=%d next=%v", out.Total, out.NextOffset)
	}
	e.call(t, "search_sessions", map[string]any{"query": "secret", "account": "work", "limit": 2, "offset": 2}, &out)
	if len(out.Results) != 1 || out.NextOffset != nil || out.Results[0].ID[:8] != "cccc0005" {
		t.Errorf("page 2: %+v", out)
	}
	e.call(t, "search_sessions", map[string]any{"query": "secret", "project": "client"}, &out)
	if out.Total != 2 {
		t.Errorf("project filter: total=%d", out.Total)
	}

	res, _ := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_sessions", Arguments: map[string]any{"query": "  "}})
	if !res.IsError {
		t.Error("blank query must be a tool error")
	}
}

func summaries(rs []SearchHit) []Summary {
	var out []Summary
	for _, r := range rs {
		out = append(out, r.Summary)
	}
	return out
}

func TestGetSession(t *testing.T) {
	e := setup(t)
	var out GetOutput
	e.call(t, "get_session", map[string]any{"id": "2222"}, &out)
	if out.Session.ID != "22222222-2222-3333-4444-555555555555" || out.Total != 5 || len(out.Messages) != 5 || out.NextOffset != nil {
		t.Fatalf("prefix lookup: %+v", out)
	}
	tu := out.Messages[1].Blocks[2]
	if tu.Type != "tool_use" || tu.Name != "Grep" {
		t.Errorf("tool_use block: %+v", tu)
	}
	if in, ok := tu.Input.(map[string]any); !ok || in["pattern"] != "needle" {
		t.Errorf("tool_use input should be an object: %#v", tu.Input)
	}
	if b := out.Messages[3].Blocks[0]; b.Type != "tool_result" || !b.IsError || b.Text != "permission denied" {
		t.Errorf("tool_result: %+v", b)
	}
	if b := out.Messages[1].Blocks[0]; b.Type != "thinking" || b.Text != "where is it" {
		t.Errorf("thinking: %+v", b)
	}

	e.call(t, "get_session", map[string]any{"id": "2222", "offset": 1, "limit": 2, "max_chars": 5}, &out)
	if out.Offset != 1 || len(out.Messages) != 2 || out.Messages[0].Index != 1 || out.NextOffset == nil || *out.NextOffset != 3 {
		t.Errorf("paging: offset=%d next=%v msgs=%+v", out.Offset, out.NextOffset, out.Messages)
	}
	if got := out.Messages[0].Blocks[0].Text; got != "where[… 6 more chars]" {
		t.Errorf("truncated thinking = %q", got)
	}
	if s, ok := out.Messages[0].Blocks[2].Input.(string); !ok || !strings.HasSuffix(s, "more chars]") {
		t.Errorf("long input becomes a cut string: %#v", out.Messages[0].Blocks[2].Input)
	}

	// Same lookup as `unpause open`: Name, then Title substring.
	e.call(t, "get_session", map[string]any{"id": "login-work", "limit": 1}, &out)
	if out.Session.ID[:8] != "11111111" || out.Session.Title != "login-work" {
		t.Errorf("name lookup: %+v", out.Session)
	}

	res, _ := e.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_session", Arguments: map[string]any{"id": "ffff"}})
	if !res.IsError || !strings.Contains(textOf(res), "not found") {
		t.Errorf("unknown id: %s", textOf(res))
	}
}

func TestToolsNeverWriteTranscripts(t *testing.T) {
	e := setup(t)
	before := snapshot(t, e.dir)
	var l ListOutput
	var s SearchOutput
	var g GetOutput
	e.call(t, "list_sessions", map[string]any{"limit": 100}, &l)
	e.call(t, "search_sessions", map[string]any{"query": "e"}, &s)
	for _, sum := range l.Sessions {
		e.call(t, "get_session", map[string]any{"id": sum.ID}, &g)
	}
	after := snapshot(t, e.dir)
	if len(before) != len(after) {
		t.Fatalf("file count changed: %d -> %d", len(before), len(after))
	}
	for p, b := range before {
		if !bytes.Equal(b, after[p]) {
			t.Errorf("%s changed", p)
		}
	}
}

func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = b
	}
	return out
}

func TestSummaryMarksCowork(t *testing.T) {
	if !summarize(&session.Session{ID: "c0c0c0c0", Cowork: true}).Cowork {
		t.Error("a Cowork Session's summary must say cowork")
	}
	if summarize(&session.Session{ID: "a0a0a0a0"}).Cowork {
		t.Error("only Cowork Sessions are marked")
	}
}
