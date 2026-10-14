package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hamzafer/unpause/internal/opener"
	"github.com/hamzafer/unpause/internal/recap"
	"github.com/hamzafer/unpause/internal/session"
)

type fakeOpener struct{ got []opener.Launch }

func (f *fakeOpener) Name() string               { return "fake" }
func (f *fakeOpener) Open(l opener.Launch) error { f.got = append(f.got, l); return nil }

func sessions() []*session.Session {
	return []*session.Session{
		{Provider: "claude", ID: "aaaaaaaa-1", Account: "work", Root: "/r/work", CWD: "/x/surgery", CustomTitle: "tracker", LastActive: time.Now(), Messages: 4,
			Preview: []session.Message{{Role: "user", Text: "fix the tracker"}, {Role: "assistant", Text: "done"}}},
		{Provider: "claude", ID: "bbbbbbbb-2", Account: "personal", Root: "/r/personal", CWD: "/x/track-one", AITitle: "Wishlist", LastActive: time.Now().Add(-3 * time.Hour), Messages: 2, ContextTokens: 140_000},
		{Provider: "claude", ID: "cccccccc-3", Account: "personal", Root: "/r/personal", CWD: "/x/gone", FirstPrompt: "hello", CWDMissing: true, LastActive: time.Now().Add(-48 * time.Hour), Messages: 1,
			Live: &session.Live{PID: 1, Status: "idle"}},
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func drive(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func TestViewListsAndFilters(t *testing.T) {
	fo := &fakeOpener{}
	m := drive(newModel(Options{Sessions: sessions(), Opener: fo, Claude: "claude"}), tea.WindowSizeMsg{Width: 140, Height: 40})
	v := m.View()
	for _, want := range []string{"unpause", "3 sessions", "★ tracker", "Wishlist", "surgery", "track-one", "work", "personal"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}
	if !strings.Contains(v, "fix the tracker") {
		t.Errorf("preview missing for first row\n%s", v)
	}
	if strings.Count(v, "◷") != 1 {
		t.Errorf("exactly one row (big + idle) should carry the summary marker\n%s", v)
	}
	m = drive(m, key("wish"))
	v = m.View()
	if strings.Contains(v, "★ tracker") || !strings.Contains(v, "Wishlist") || !strings.Contains(v, "1 match") {
		t.Errorf("filter failed\n%s", v)
	}
}

func TestRetiredWorktreeIsCalmAndOpensInRepo(t *testing.T) {
	repo := t.TempDir()
	s := &session.Session{Provider: "claude", ID: "dddddddd-4", Account: "work", Root: "/r/work", CWD: repo + "/.claude/worktrees/pow-796", AITitle: "Proof of work",
		CWDMissing: true, WorktreeRoot: repo, LastActive: time.Now(), Messages: 2}
	fo := &fakeOpener{}
	m := drive(newModel(Options{Sessions: []*session.Session{s}, Opener: fo, Claude: "claude"}), tea.WindowSizeMsg{Width: 140, Height: 30})
	v := m.View()
	if !strings.Contains(v, "⎇ Proof of work") || !strings.Contains(v, "⎇ worktree removed; will open in") {
		t.Errorf("retired worktree not shown\n%s", v)
	}
	if strings.Contains(v, "! folder no longer exists") {
		t.Errorf("retired worktree must not look like an orphan\n%s", v)
	}
	drive(m, key("enter"))
	if len(fo.got) != 1 || fo.got[0].CWD != repo {
		t.Errorf("expected launch in %s, got %+v", repo, fo.got)
	}
}

func TestEnterOpensWithRightAccount(t *testing.T) {
	fo := &fakeOpener{}
	m := drive(newModel(Options{Sessions: sessions(), Opener: fo, Claude: "claude"}), tea.WindowSizeMsg{Width: 120, Height: 30}, key("down"), key("enter"))
	if len(fo.got) != 1 {
		t.Fatalf("opened %d", len(fo.got))
	}
	l := fo.got[0]
	if l.SessionID != "bbbbbbbb-2" || l.ConfigDir != "/r/personal" || l.CWD != "/x/track-one" || l.Fork {
		t.Errorf("launch = %+v", l)
	}
	if !strings.Contains(m.View(), "opened") {
		t.Errorf("status missing\n%s", m.View())
	}
}

func TestLiveSessionAsksBeforeOpening(t *testing.T) {
	fo := &fakeOpener{}
	m := drive(newModel(Options{Sessions: sessions(), Opener: fo, Claude: "claude"}), tea.WindowSizeMsg{Width: 120, Height: 30}, key("down"), key("down"), key("enter"))
	if len(fo.got) != 0 {
		t.Fatal("must not open a live session without confirmation")
	}
	if !strings.Contains(m.View(), "running right now") {
		t.Errorf("confirm prompt missing\n%s", m.View())
	}
	m = drive(m, key("f"))
	if len(fo.got) != 1 || !fo.got[0].Fork {
		t.Errorf("expected fork launch, got %+v", fo.got)
	}
	if fo.got[0].CWD == "/x/gone" {
		t.Error("orphan must not open in a missing cwd")
	}
}

func TestInPlaceOpenerQuitsWithLaunch(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: opener.InPlace{}, Claude: "claude"}), tea.WindowSizeMsg{Width: 120, Height: 30})
	next, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if next.(model).result.Launch == nil || next.(model).result.Launch.SessionID != "aaaaaaaa-1" {
		t.Errorf("launch not handed back: %+v", next.(model).result)
	}
}

func TestRenameFlow(t *testing.T) {
	var renamed string
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Rename: func(s *session.Session, n string) error { renamed = n; return nil }}),
		tea.WindowSizeMsg{Width: 120, Height: 30}, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlU}) // clear existing name
	for _, r := range "new-name" {
		m = drive(m, key(string(r)))
	}
	m = drive(m, key("enter"))
	if renamed != "new-name" {
		t.Errorf("renamed = %q", renamed)
	}
	if !strings.Contains(m.View(), "★ new-name") {
		t.Errorf("list not updated\n%s", m.View())
	}
}

func TestFilterIsTermSubstringNotSubsequence(t *testing.T) {
	if matches("try to reach via built in surgery-planning work", "track") {
		t.Error("subsequence match must not count")
	}
	if !matches("wishlist contents track-one personal main bbbbbbbb", "track pers") {
		t.Error("every term as substring must match")
	}
}

func TestEscQuitsWhenFilterEmpty(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude"}), tea.WindowSizeMsg{Width: 120, Height: 30})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc on empty filter must quit")
	}
	next, cmd := m.Update(key("x"))
	if _, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
		t.Fatal("esc with filter must only clear")
	}
}

func TestCoworkSessionIsReadOnly(t *testing.T) {
	s := &session.Session{Provider: "claude", ID: "eeeeeeee-5", Account: "cowork", Root: "/r/cowork", CWD: "/sessions/x", DesktopTitle: "Make a deck", Cowork: true, LastActive: time.Now(), Messages: 2}
	fo := &fakeOpener{}
	renamed := false
	m := drive(newModel(Options{Sessions: []*session.Session{s}, Opener: fo, Claude: "claude", Rename: func(*session.Session, string) error { renamed = true; return nil }}),
		tea.WindowSizeMsg{Width: 140, Height: 30}, key("enter"))
	if len(fo.got) != 0 {
		t.Fatalf("cowork session must not open, got %+v", fo.got)
	}
	if !strings.Contains(m.View(), "Cowork session: open it in Claude Desktop") {
		t.Errorf("status missing\n%s", m.View())
	}
	m = drive(m, key("ctrl+f"))
	if len(fo.got) != 0 {
		t.Fatalf("cowork session must not fork, got %+v", fo.got)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlR}, key("x"), key("enter"))
	if renamed || m.(model).mode == modeRename {
		t.Error("cowork session must not be renamed")
	}
}

// fakeRecaps stands in for the recap store: only the first Session has a Recap.
func fakeRecaps(s *session.Session) (recap.Recap, bool, bool) {
	if s.ID != "aaaaaaaa-1" {
		return recap.Recap{}, false, false
	}
	return recap.Recap{Goal: "keep the hertz tracker honest", Done: "tracker fixed", Next: "ship it"}, false, true
}

func TestPreviewShowsRecapBoxBetweenMetadataAndMessages(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Recap: fakeRecaps}), tea.WindowSizeMsg{Width: 140, Height: 40})
	v := m.View()
	for _, want := range []string{"goal", "keep the hertz tracker honest", "done", "tracker fixed", "next", "ship it", "╭"} {
		if !strings.Contains(v, want) {
			t.Errorf("recap box missing %q\n%s", want, v)
		}
	}
	meta, box, msgs := strings.Index(v, "4 messages"), strings.Index(v, "keep the hertz tracker honest"), strings.Index(v, "fix the tracker")
	if !(meta < box && box < msgs) {
		t.Errorf("box must sit between metadata (%d) and messages (%d), at %d\n%s", meta, msgs, box, v)
	}
	// The next Session has no Recap: no box.
	if v := drive(m, key("down")).View(); strings.Contains(v, "╭") || strings.Contains(v, "hertz") {
		t.Errorf("a Session without a Recap must show no box\n%s", v)
	}
}

func TestNoRecapBoxWhenRecapsAreOff(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude"}), tea.WindowSizeMsg{Width: 140, Height: 40})
	if v := m.View(); strings.Contains(v, "╭") || strings.Contains(v, "hertz tracker honest") {
		t.Errorf("Recaps off must show no box\n%s", v)
	}
	if v := m.View(); strings.Contains(v, "^t") {
		t.Errorf("Recaps off must not offer ^t\n%s", v)
	}
	// ^t does nothing when Recaps are off.
	if v := drive(m, tea.KeyMsg{Type: tea.KeyCtrlT}).View(); strings.Contains(v, "╭") {
		t.Errorf("^t must not show a box when Recaps are off\n%s", v)
	}
}

func TestOutdatedRecapIsMarkedEarlier(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Recap: fakeRecaps}), tea.WindowSizeMsg{Width: 140, Height: 40})
	if v := m.View(); strings.Contains(v, "earlier") {
		t.Errorf("a current Recap must not say earlier\n%s", v)
	}
	outdated := func(s *session.Session) (recap.Recap, bool, bool) {
		r, _, ok := fakeRecaps(s)
		return r, true, ok
	}
	m = drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Recap: outdated}), tea.WindowSizeMsg{Width: 140, Height: 40})
	if v := m.View(); !strings.Contains(v, "goal · earlier") || !strings.Contains(v, "keep the hertz tracker honest") {
		t.Errorf("an outdated Recap must say · earlier after the label\n%s", v)
	}
}

// Looking a Recap up checks the transcript on disk, so the picker does it once per
// Session, not on every render.
func TestRecapIsLookedUpOncePerSession(t *testing.T) {
	calls := map[string]int{}
	counted := func(s *session.Session) (recap.Recap, bool, bool) {
		calls[s.ID]++
		return fakeRecaps(s)
	}
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Recap: counted}), tea.WindowSizeMsg{Width: 140, Height: 40})
	for range 3 {
		m.View()
	}
	m = drive(m, key("down"), key("up"))
	m.View()
	for id, n := range calls {
		if n != 1 {
			t.Errorf("Recap for %s looked up %d times, want once", id, n)
		}
	}
	if calls["aaaaaaaa-1"] != 1 {
		t.Errorf("the shown Session's Recap was never looked up: %v", calls)
	}
}

func TestCtrlTTogglesTheRecapBox(t *testing.T) {
	m := drive(newModel(Options{Sessions: sessions(), Opener: &fakeOpener{}, Claude: "claude", Recap: fakeRecaps}), tea.WindowSizeMsg{Width: 140, Height: 40})
	if v := m.View(); !strings.Contains(v, "^t") || !strings.Contains(v, "recap") {
		t.Errorf("footer must offer ^t recap when Recaps are on\n%s", v)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlT})
	if v := m.View(); strings.Contains(v, "hertz tracker honest") {
		t.Errorf("^t must hide the box\n%s", v)
	}
	if v := drive(m, key("down"), key("up")).View(); strings.Contains(v, "hertz tracker honest") {
		t.Errorf("the box stays hidden while moving\n%s", v)
	}
	if v := drive(m, tea.KeyMsg{Type: tea.KeyCtrlT}).View(); !strings.Contains(v, "hertz tracker honest") {
		t.Errorf("^t again must show the box\n%s", v)
	}
}
