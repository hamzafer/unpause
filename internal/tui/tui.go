// Package tui is the interactive picker: a filterable list of sessions with a preview pane.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/opener"
	"github.com/hamzafer/unpause/internal/session"
)

// Options configures the picker.
type Options struct {
	Sessions []*session.Session
	Opener   opener.Opener
	Claude   string
	Rename   func(s *session.Session, name string) error
}

// Result is what the picker hands back when it exits.
type Result struct {
	// Launch is set when the chosen opener must run after the TUI has released the terminal.
	Launch *opener.Launch
}

type mode int

const (
	modeList mode = iota
	modeRename
	modeLiveConfirm
)

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#a78bfa"}
	colDim    = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#9ca3af"}
	colLive   = lipgloss.AdaptiveColor{Light: "#16a34a", Dark: "#4ade80"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#d97706", Dark: "#fbbf24"}
	colErr    = lipgloss.AdaptiveColor{Light: "#dc2626", Dark: "#f87171"}

	stTitle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stDim      = lipgloss.NewStyle().Foreground(colDim)
	stLive     = lipgloss.NewStyle().Foreground(colLive)
	stWarn     = lipgloss.NewStyle().Foreground(colWarn)
	stErr      = lipgloss.NewStyle().Foreground(colErr)
	stCursor   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stSelected = lipgloss.NewStyle().Bold(true)
	stAccount  = map[string]lipgloss.Style{}
	stKey      = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stUser     = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stAsst     = lipgloss.NewStyle().Bold(true).Foreground(colLive)
	stPane     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(colDim).BorderLeft(true).PaddingLeft(1)
)

var accountPalette = []lipgloss.AdaptiveColor{
	{Light: "#2563eb", Dark: "#60a5fa"},
	{Light: "#db2777", Dark: "#f472b6"},
	{Light: "#0d9488", Dark: "#2dd4bf"},
	{Light: "#ea580c", Dark: "#fb923c"},
}

type model struct {
	opts     Options
	all      []*session.Session
	visible  []int // indexes into all
	cursor   int
	offset   int
	query    textinput.Model
	rename   textinput.Model
	mode     mode
	width    int
	height   int
	status   string
	statusOK bool
	result   Result
	accounts []string
}

// Run starts the picker and blocks until it exits.
func Run(o Options) (Result, error) {
	m := newModel(o)
	p := tea.NewProgram(m, tea.WithAltScreen())
	out, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	return out.(model).result, nil
}

func newModel(o Options) model {
	q := textinput.New()
	q.Placeholder = "type to filter  ·  name, repo, account, branch, id"
	q.Prompt = "› "
	q.PromptStyle = stKey
	q.Focus()

	r := textinput.New()
	r.Prompt = "name › "
	r.PromptStyle = stKey
	r.CharLimit = 80

	seen := map[string]bool{}
	var accounts []string
	for _, s := range o.Sessions {
		if !seen[s.Account] {
			seen[s.Account] = true
			accounts = append(accounts, s.Account)
		}
	}
	sort.Strings(accounts)
	for i, a := range accounts {
		stAccount[a] = lipgloss.NewStyle().Foreground(accountPalette[i%len(accountPalette)])
	}

	m := model{opts: o, all: o.Sessions, query: q, rename: r, accounts: accounts}
	m.refilter()
	return m
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m *model) current() *session.Session {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.all[m.visible[m.cursor]]
}

func haystack(s *session.Session) string {
	return strings.ToLower(strings.Join([]string{s.Title(), s.Repo(), s.Account, s.Branch, s.ShortID()}, " "))
}

// matches reports whether every whitespace-separated term of q appears somewhere in hay.
func matches(hay, q string) bool {
	for _, term := range strings.Fields(strings.ToLower(q)) {
		if !strings.Contains(hay, term) {
			return false
		}
	}
	return true
}

func (m *model) refilter() {
	q := strings.TrimSpace(m.query.Value())
	m.visible = m.visible[:0]
	for i, s := range m.all {
		if q == "" || matches(haystack(s), q) {
			m.visible = append(m.visible, i)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.clampOffset()
}

func (m *model) listHeight() int {
	// header(1) + blank(1) + filter(1) + blank(1) + list + blank(1) + footer(1)
	return max(3, m.height-6)
}

func (m *model) clampOffset() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) move(d int) {
	if len(m.visible) == 0 {
		return
	}
	m.cursor = (m.cursor + d + len(m.visible)) % len(m.visible)
	m.clampOffset()
}

func (m *model) setStatus(s string, ok bool) {
	m.status, m.statusOK = s, ok
}

func (m *model) launch(s *session.Session, fork bool) (tea.Model, tea.Cmd) {
	l := opener.Launch{
		Claude:    m.opts.Claude,
		SessionID: s.ID,
		CWD:       s.CWD,
		ConfigDir: config.EnvFor(s.Root),
		Title:     s.Title(),
		Fork:      fork,
	}
	if s.CWDMissing || s.CWD == "" {
		// Orphan: open in the home dir; Claude Code resumes the transcript regardless of cwd.
		l.CWD = homeDir()
	}
	if opener.NeedsDetach(m.opts.Opener) {
		m.result.Launch = &l
		return *m, tea.Quit
	}
	if err := m.opts.Opener.Open(l); err != nil {
		m.setStatus(err.Error(), false)
		return *m, nil
	}
	verb := "opened"
	if fork {
		verb = "forked"
	}
	m.setStatus(fmt.Sprintf("%s %q via %s", verb, session.Clip(s.Title(), 40), m.opts.Opener.Name()), true)
	return *m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampOffset()
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeRename:
			return m.updateRename(msg)
		case modeLiveConfirm:
			return m.updateLiveConfirm(msg)
		}
		return m.updateList(msg)
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.query.Value() != "" {
			m.query.SetValue("")
			m.refilter()
			return m, nil
		}
		return m, tea.Quit
	case "up", "ctrl+p", "ctrl+k":
		m.move(-1)
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.move(1)
		return m, nil
	case "pgup":
		m.move(-m.listHeight())
		return m, nil
	case "pgdown":
		m.move(m.listHeight())
		return m, nil
	case "home":
		m.cursor = 0
		m.clampOffset()
		return m, nil
	case "end":
		m.cursor = max(0, len(m.visible)-1)
		m.clampOffset()
		return m, nil
	case "enter":
		s := m.current()
		if s == nil {
			return m, nil
		}
		if s.Live != nil {
			m.mode = modeLiveConfirm
			return m, nil
		}
		return m.launch(s, false)
	case "ctrl+f":
		if s := m.current(); s != nil {
			return m.launch(s, true)
		}
		return m, nil
	case "ctrl+r":
		if s := m.current(); s != nil && m.opts.Rename != nil {
			m.mode = modeRename
			m.rename.SetValue(s.CustomTitle)
			m.rename.CursorEnd()
			m.rename.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}
	var cmd tea.Cmd
	before := m.query.Value()
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() != before {
		m.cursor = 0
		m.refilter()
	}
	return m, cmd
}

func (m model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		m.rename.Blur()
		return m, nil
	case "enter":
		s := m.current()
		name := strings.TrimSpace(m.rename.Value())
		m.mode = modeList
		m.rename.Blur()
		if s == nil || name == "" || name == s.CustomTitle {
			return m, nil
		}
		if err := m.opts.Rename(s, name); err != nil {
			m.setStatus("rename failed: "+err.Error(), false)
			return m, nil
		}
		s.CustomTitle = name
		m.setStatus(fmt.Sprintf("renamed to %q", name), true)
		m.refilter()
		return m, nil
	}
	var cmd tea.Cmd
	m.rename, cmd = m.rename.Update(msg)
	return m, cmd
}

func (m model) updateLiveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.current()
	switch msg.String() {
	case "esc", "ctrl+c", "n":
		m.mode = modeList
		return m, nil
	case "f", "enter":
		m.mode = modeList
		if s != nil {
			return m.launch(s, true)
		}
	case "o":
		m.mode = modeList
		if s != nil {
			return m.launch(s, false)
		}
	}
	return m, nil
}

// ---- view ----

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	listW := m.width * 55 / 100
	if m.width < 100 {
		listW = m.width
	}
	previewW := m.width - listW - 3

	header := stTitle.Render("unpause") + stDim.Render(fmt.Sprintf("  %d sessions · %s", len(m.all), strings.Join(m.accounts, ", ")))
	if q := strings.TrimSpace(m.query.Value()); q != "" {
		header += stDim.Render(fmt.Sprintf(" · %d match", len(m.visible)))
	}

	var input string
	switch m.mode {
	case modeRename:
		input = m.rename.View()
	default:
		input = m.query.View()
	}

	list := m.renderList(listW)
	var body string
	if previewW >= 30 {
		preview := stPane.Width(previewW).Height(m.listHeight()).Render(m.renderPreview(previewW - 2))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listW).Render(list), preview)
	} else {
		body = list
	}

	return strings.Join([]string{header, "", input, "", body, "", m.renderFooter()}, "\n")
}

func (m model) renderList(w int) string {
	h := m.listHeight()
	if len(m.visible) == 0 {
		return stDim.Render("  nothing matches")
	}
	lines := make([]string, 0, h)
	end := min(len(m.visible), m.offset+h)
	// column widths
	repoW := 18
	if w < 80 {
		repoW = 12
	}
	titleW := w - 2 - 2 - repoW - 1 - 9 - 1 - 4 - 1
	if titleW < 10 {
		titleW = 10
	}
	for i := m.offset; i < end; i++ {
		s := m.all[m.visible[i]]
		cur := i == m.cursor
		prefix := "  "
		if cur {
			prefix = stCursor.Render("▶ ")
		}
		dot := " "
		switch {
		case s.Live != nil:
			dot = stLive.Render("●")
		case s.CWDMissing:
			dot = stWarn.Render("!")
		}
		title := session.Clip(s.Title(), titleW)
		if s.Named() {
			title = "★ " + session.Clip(s.Title(), titleW-2)
		}
		title = fmt.Sprintf("%-*s", titleW, title)
		if cur {
			title = stSelected.Render(title)
		}
		acct := stAccount[s.Account].Render(fmt.Sprintf("%-8s", session.Clip(s.Account, 8)))
		repo := stDim.Render(fmt.Sprintf("%-*s", repoW, session.Clip(s.Repo(), repoW)))
		ageS := stDim.Render(fmt.Sprintf("%4s", session.Age(s.LastActive)))
		lines = append(lines, prefix+dot+" "+title+" "+repo+" "+acct+" "+ageS)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) renderPreview(w int) string {
	s := m.current()
	if s == nil {
		return ""
	}
	var b strings.Builder
	wrap := lipgloss.NewStyle().Width(w)
	b.WriteString(wrap.Render(stTitle.Render(s.Title())))
	b.WriteString("\n")
	meta := []string{
		stDim.Render("id      ") + s.ShortID(),
		stDim.Render("account ") + stAccount[s.Account].Render(s.Account),
		stDim.Render("repo    ") + s.CWD,
	}
	if s.Branch != "" {
		meta = append(meta, stDim.Render("branch  ")+s.Branch)
	}
	active := session.Age(s.LastActive)
	if active != "now" {
		active += " ago"
	}
	meta = append(meta, stDim.Render("active  ")+active+stDim.Render(fmt.Sprintf("  ·  %d messages", s.Messages)))
	if s.Live != nil {
		meta = append(meta, stLive.Render("● running")+stDim.Render(fmt.Sprintf("  pid %d  %s", s.Live.PID, s.Live.Status)))
	}
	if s.CWDMissing {
		meta = append(meta, stWarn.Render("! folder no longer exists; will open in ~"))
	}
	for _, l := range meta {
		b.WriteString(wrap.Render(l))
		b.WriteString("\n")
	}

	if m.mode == modeLiveConfirm {
		b.WriteString("\n")
		b.WriteString(stWarn.Render("This session is running right now."))
		b.WriteString("\n")
		b.WriteString(stKey.Render("f") + " fork a copy   " + stKey.Render("o") + " open anyway   " + stKey.Render("esc") + " cancel")
		return b.String()
	}

	b.WriteString("\n")
	avail := m.listHeight() - len(meta) - 3
	msgs := s.Preview
	// Show as many trailing messages as fit, each capped to a few lines.
	var chunks []string
	used := 0
	for i := len(msgs) - 1; i >= 0 && used < avail; i-- {
		msg := msgs[i]
		label := stAsst.Render("claude")
		if msg.Role == "user" {
			label = stUser.Render("you")
		}
		text := wrap.Render(session.Clip(msg.Text, w*4))
		lines := strings.Split(text, "\n")
		if len(lines) > 5 {
			lines = append(lines[:5], stDim.Render("…"))
		}
		block := label + "\n" + strings.Join(lines, "\n")
		n := len(lines) + 2
		if used+n > avail && len(chunks) > 0 {
			break
		}
		chunks = append([]string{block}, chunks...)
		used += n
	}
	b.WriteString(strings.Join(chunks, "\n\n"))
	return b.String()
}

func (m model) renderFooter() string {
	if m.status != "" {
		st := stLive
		if !m.statusOK {
			st = stErr
		}
		return st.Render(session.Clip(m.status, m.width-1))
	}
	switch m.mode {
	case modeRename:
		return stKey.Render("enter") + stDim.Render(" save  ") + stKey.Render("esc") + stDim.Render(" cancel")
	case modeLiveConfirm:
		return stKey.Render("f") + stDim.Render(" fork  ") + stKey.Render("o") + stDim.Render(" open anyway  ") + stKey.Render("esc") + stDim.Render(" cancel")
	}
	keys := []string{
		stKey.Render("enter") + stDim.Render(" open"),
		stKey.Render("^f") + stDim.Render(" fork"),
		stKey.Render("^r") + stDim.Render(" rename"),
		stKey.Render("↑↓") + stDim.Render(" move"),
		stKey.Render("esc") + stDim.Render(" clear/quit"),
	}
	return strings.Join(keys, "  ") + stDim.Render("   opens via "+m.opts.Opener.Name())
}
