package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hamzafer/unpause/internal/opener"
	"github.com/hamzafer/unpause/internal/session"
)

type fakeOpener struct{ got []opener.Launch }

func (f *fakeOpener) Name() string               { return "fake" }
func (f *fakeOpener) Open(l opener.Launch) error { f.got = append(f.got, l); return nil }

func sessions() []*session.Session {
	return []*session.Session{
		{Provider: "claude", ID: "aaaaaaaa-1", Account: "work", Root: "/r/work", CWD: "/x/surgery", CustomTitle: "tracker", LastActive: time.Now(), Messages: 4,
			Preview: []session.Message{{Role: "user", Text: "fix the tracker"}, {Role: "assistant", Text: "done"}}},
		{Provider: "claude", ID: "bbbbbbbb-2", Account: "personal", Root: "/r/personal", CWD: "/x/track-one", AITitle: "Wishlist", LastActive: time.Now().Add(-time.Hour), Messages: 2},
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
	m = drive(m, key("wish"))
	v = m.View()
	if strings.Contains(v, "★ tracker") || !strings.Contains(v, "Wishlist") || !strings.Contains(v, "1 match") {
		t.Errorf("filter failed\n%s", v)
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
