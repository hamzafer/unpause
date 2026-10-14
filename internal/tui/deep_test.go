package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/session"
)

type fakeSearch struct {
	queries []string
	results map[string][]claude.Result
	err     error
}

func (f *fakeSearch) search(ctx context.Context, _ []*session.Session, q string, progress func(done, total int)) ([]claude.Result, error) {
	f.queries = append(f.queries, q)
	if progress != nil {
		progress(1, 3)
	}
	return f.results[q], f.err
}

func deepModel(fs *fakeSearch) (tea.Model, []*session.Session) {
	ss := sessions()
	if fs.results == nil {
		fs.results = map[string][]claude.Result{
			"tunnel": {
				{Session: ss[1], Hits: 12, Snippets: []string{"…store the runtime key in Keychain for the Tunnel…", "the tunnel is up"}},
				{Session: ss[0], Hits: 3, Snippets: []string{"a tunnel"}},
			},
		}
	}
	m := drive(newModel(Options{Sessions: ss, Opener: &fakeOpener{}, Claude: "claude", Search: fs.search}), tea.WindowSizeMsg{Width: 140, Height: 40})
	return m, ss
}

// runCmd executes cmd and everything it batches, returning the messages produced.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func typeText(m tea.Model, s string) (tea.Model, []tea.Cmd) {
	var cmds []tea.Cmd
	for _, r := range s {
		var cmd tea.Cmd
		m, cmd = m.Update(key(string(r)))
		cmds = append(cmds, cmd)
	}
	return m, cmds
}

// settle feeds Deep search messages back into the model until none are left.
func settle(m tea.Model, msgs []tea.Msg) tea.Model {
	for len(msgs) > 0 {
		msg := msgs[0]
		msgs = msgs[1:]
		switch msg.(type) {
		case debounceMsg, searchDoneMsg, progressMsg:
			var cmd tea.Cmd
			m, cmd = m.Update(msg)
			msgs = append(msgs, runCmd(cmd)...)
		}
	}
	return m
}

func TestDeepSearchDebounceRunsOneSearch(t *testing.T) {
	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, cmds := typeText(m, "tunnel")
	if len(fs.queries) != 0 {
		t.Fatal("search must wait for the debounce")
	}
	// Every keystroke's timer fires; only the last one may search.
	var msgs []tea.Msg
	for _, c := range cmds {
		msgs = append(msgs, runCmd(c)...)
	}
	m = settle(m, msgs)
	if len(fs.queries) != 1 || fs.queries[0] != "tunnel" {
		t.Fatalf("queries = %q, want exactly one for \"tunnel\"", fs.queries)
	}
	if v := m.View(); !strings.Contains(v, "deep · 2 match") {
		t.Errorf("header missing match count\n%s", v)
	}
}

func TestDeepSearchNewKeystrokeCancelsRunningSearch(t *testing.T) {
	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "tun")
	m, _ = m.Update(debounceMsg{id: m.(model).searchID})
	run := m.(model).run
	if run == nil || run.ctx.Err() != nil {
		t.Fatal("search should be running")
	}
	if v := m.View(); !strings.Contains(v, "searching 0/3") {
		t.Errorf("progress header missing\n%s", v)
	}
	m, _ = typeText(m, "n")
	if run.ctx.Err() == nil {
		t.Error("a new keystroke must cancel the running search")
	}
}

func TestDeepSearchStaleResultIgnored(t *testing.T) {
	fs := &fakeSearch{}
	m, ss := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "tun")
	old := m.(model).searchID
	m, _ = typeText(m, "nel")
	m = drive(m, searchDoneMsg{id: old, results: []claude.Result{{Session: ss[2], Hits: 99}}})
	if v := m.View(); strings.Contains(v, "99×") {
		t.Errorf("stale result shown\n%s", v)
	}
	m = drive(m, searchDoneMsg{id: m.(model).searchID, results: fs.results["tunnel"]})
	if v := m.View(); !strings.Contains(v, "12×") {
		t.Errorf("fresh result missing\n%s", v)
	}
}

func TestDeepSearchRankedListAndPreview(t *testing.T) {
	old := renderHit
	renderHit = func(s string) string { return "[" + s + "]" }
	defer func() { renderHit = old }()

	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "tunnel")
	m = settle(m, []tea.Msg{debounceMsg{id: m.(model).searchID}})
	m = drive(m, key("down"))
	// A new search result puts the cursor back on top.
	m = drive(m, searchDoneMsg{id: m.(model).searchID, results: fs.results["tunnel"]})
	v := m.View()
	for _, want := range []string{"deep › ", "12×", " 3×", "Wishlist", "★ tracker", "[Tunnel]", "[tunnel] is up", "^s back"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}
	if strings.Contains(v, "hello") {
		t.Errorf("non-matching session listed\n%s", v)
	}
	if strings.Index(v, "Wishlist") > strings.Index(v, "★ tracker") {
		t.Errorf("results not ranked\n%s", v)
	}
	mm := m.(model)
	if cur := mm.current(); cur == nil || cur.ID != "bbbbbbbb-2" {
		t.Errorf("cursor not at top: %+v", cur)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 140 {
			t.Errorf("line too wide (%d): %q", w, line)
		}
	}
	// The first result's preview shows snippets, not the last messages.
	m = drive(m, key("down"))
	if v := m.View(); strings.Contains(v, "fix the tracker") || !strings.Contains(v, "a [tunnel]") {
		t.Errorf("preview should show snippets, not the last messages\n%s", v)
	}
}

func TestDeepSearchCarriesFilterAndEscReturns(t *testing.T) {
	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m, _ = typeText(m, "tunnel")
	if v := m.View(); !strings.Contains(v, "^s deep search") {
		t.Errorf("footer missing ^s hint\n%s", v)
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if got := m.(model).query.Value(); got != "tunnel" {
		t.Errorf("filter text not carried over: %q", got)
	}
	m = settle(m, runCmd(cmd))
	if len(fs.queries) != 1 {
		t.Fatalf("carried query should search once, got %q", fs.queries)
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("esc in deep search must not quit")
		}
	}
	mm := m.(model)
	if mm.deep || mm.query.Value() != "" {
		t.Errorf("esc must return to an empty normal filter: deep=%v query=%q", mm.deep, mm.query.Value())
	}
	v := m.View()
	for _, want := range []string{"★ tracker", "Wishlist", "hello", "^s deep search"} {
		if !strings.Contains(v, want) {
			t.Errorf("normal list not restored, missing %q\n%s", want, v)
		}
	}
	if strings.Contains(v, "deep ›") || strings.Contains(v, "12×") {
		t.Errorf("still in deep mode\n%s", v)
	}
}

func TestDeepSearchCtrlSTogglesBack(t *testing.T) {
	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.(model).deep {
		t.Fatal("^s must enter deep search")
	}
	if v := m.View(); !strings.Contains(v, "search inside transcripts") {
		t.Errorf("deep placeholder missing\n%s", v)
	}
	m, _ = typeText(m, "x")
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if mm := m.(model); mm.deep || mm.query.Value() != "" || len(mm.visible) != 3 {
		t.Errorf("^s must go back to the full normal list: deep=%v query=%q visible=%d", mm.deep, mm.query.Value(), len(mm.visible))
	}
}

func TestDeepSearchRenameKeepsResults(t *testing.T) {
	ss := sessions()
	fs := &fakeSearch{results: map[string][]claude.Result{"tunnel": {{Session: ss[1], Hits: 2, Snippets: []string{"tunnel"}}}}}
	m := drive(newModel(Options{Sessions: ss, Opener: &fakeOpener{}, Claude: "claude", Search: fs.search, Rename: func(*session.Session, string) error { return nil }}),
		tea.WindowSizeMsg{Width: 140, Height: 40}, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "tunnel")
	m = settle(m, []tea.Msg{debounceMsg{id: m.(model).searchID}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlR}, tea.KeyMsg{Type: tea.KeyCtrlU})
	m, _ = typeText(m, "fresh")
	m = drive(m, key("enter"))
	v := m.View()
	if !strings.Contains(v, "★ fresh") || !strings.Contains(v, "2×") || !m.(model).deep {
		t.Errorf("results should stay after rename\n%s", v)
	}
}

func TestDeepSearchEmptyQueryShowsHint(t *testing.T) {
	fs := &fakeSearch{}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, cmds := typeText(m, "a")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	var msgs []tea.Msg
	for _, c := range append(cmds, cmd) {
		msgs = append(msgs, runCmd(c)...)
	}
	m = settle(m, msgs)
	if len(fs.queries) != 0 {
		t.Errorf("empty query must not search, got %q", fs.queries)
	}
	if v := m.View(); !strings.Contains(v, "type to search") {
		t.Errorf("hint missing\n%s", v)
	}
}

func TestHighlightIsCaseInsensitiveAndRuneSafe(t *testing.T) {
	hl := func(s string) string { return "[" + s + "]" }
	cases := []struct{ in, want string }{
		{"Tunnel and TUNNEL", "[Tunnel] and [TUNNEL]"},
		{"…æøå Tunnel…", "…æøå [Tunnel]…"},
		{"key keychain", "[key] [keychain]"},
		{"nothing here", "nothing here"},
		{"İstanbul tunnel", "İstanbul [tunnel]"},
	}
	for _, c := range cases {
		if got := highlight(c.in, []string{"tunnel", "key", "keychain"}, hl); got != c.want {
			t.Errorf("highlight(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDeepSearchFailureSaysSo(t *testing.T) {
	fs := &fakeSearch{err: errors.New("disk on fire")}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, cmds := typeText(m, "x")
	var msgs []tea.Msg
	for _, c := range cmds {
		msgs = append(msgs, runCmd(c)...)
	}
	m = settle(m, msgs)
	v := m.View()
	if !strings.Contains(v, "search failed") || strings.Contains(v, "searching…") {
		t.Errorf("failure not shown cleanly\n%s", v)
	}
}

func TestDeepSearchFailureClearsOnNextQuery(t *testing.T) {
	fs := &fakeSearch{err: errors.New("disk on fire")}
	m, _ := deepModel(fs)
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "x")
	m = settle(m, runCmd(func() tea.Msg { return debounceMsg{id: m.(model).searchID} }))
	if !strings.Contains(m.View(), "  search failed") {
		t.Fatal("failure not shown")
	}
	fs.err = nil
	m, _ = typeText(m, "y")
	m = settle(m, runCmd(func() tea.Msg { return debounceMsg{id: m.(model).searchID} }))
	if v := m.View(); strings.Contains(v, "  search failed") || !strings.Contains(v, "nothing matches") {
		t.Errorf("stale failure after a good search\n%s", v)
	}
}

func TestDeepSearchReadsCopiesAndHandsBackOriginals(t *testing.T) {
	ss := sessions()
	var got []*session.Session
	search := func(_ context.Context, in []*session.Session, q string, _ func(int, int)) ([]claude.Result, error) {
		got = in
		return []claude.Result{{Session: in[1], Hits: 1}}, nil
	}
	m := drive(newModel(Options{Sessions: ss, Opener: &fakeOpener{}, Claude: "claude", Search: search}), tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "x")
	m = settle(m, runCmd(func() tea.Msg { return debounceMsg{id: m.(model).searchID} }))
	if len(got) != len(ss) || got[1] == ss[1] || got[1].ID != ss[1].ID {
		t.Fatal("search must get copies of the sessions")
	}
	if r := m.(model).results; len(r) != 1 || r[0].Session != ss[1] {
		t.Error("results must point at the picker's own sessions, so a rename shows")
	}
}

func TestDeepSearchRenameTargetSurvivesNewResults(t *testing.T) {
	fs := &fakeSearch{}
	ss := sessions()
	var renamed *session.Session
	fs.results = map[string][]claude.Result{"tunnel": {{Session: ss[0], Hits: 9}, {Session: ss[1], Hits: 1}}}
	m := drive(newModel(Options{Sessions: ss, Opener: &fakeOpener{}, Claude: "claude", Search: fs.search,
		Rename: func(s *session.Session, _ string) error { renamed = s; return nil }}), tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = typeText(m, "tunnel")
	m = settle(m, runCmd(func() tea.Msg { return debounceMsg{id: m.(model).searchID} }))
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}) // second result
	mm := m.(model)
	want := mm.current()
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	// A search finishing while the prompt is open moves the cursor to the top.
	m = settle(m, []tea.Msg{searchDoneMsg{id: m.(model).searchID, results: fs.results["tunnel"]}})
	m, _ = typeText(m, "new name")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if renamed != want {
		t.Errorf("renamed %v, want %v", renamed.ID, want.ID)
	}
}
