package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hamzafer/unpause/internal/provider/claude"
)

// Deep search: ^s turns the filter box into a search over what was said in each
// Session (ADR 0011). Typing starts a search after a debounce; a new keystroke
// cancels the running one and stale results are dropped by search id.

const (
	filterPrompt      = "› "
	filterPlaceholder = "type to filter  ·  name, repo, account, branch, id"
	deepPrompt        = "deep › "
	deepPlaceholder   = `search inside transcripts  ·  "quotes" for a phrase`
)

var (
	debounceDelay = 300 * time.Millisecond
	progressEvery = 100 * time.Millisecond

	stHit = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	// renderHit styles a matched term in a snippet. A variable so tests can see it.
	renderHit = func(s string) string { return stHit.Render(s) }
)

// debounceMsg fires debounceDelay after a keystroke; only the latest id searches.
type debounceMsg struct{ id int }

// progressMsg asks the model to redraw the running search's progress.
type progressMsg struct{ id int }

// searchDoneMsg carries a finished search's results.
type searchDoneMsg struct {
	id      int
	results []claude.Result
	err     error
}

// searchRun is one search in flight. The search goroutine writes done/total and the
// model reads them on each progress tick.
type searchRun struct {
	id          int
	ctx         context.Context
	cancel      context.CancelFunc
	done, total atomic.Int64
}

func (m *model) enterDeep() tea.Cmd {
	m.deep = true
	m.query.Prompt = deepPrompt
	m.query.Placeholder = deepPlaceholder
	m.results, m.terms, m.searched = nil, nil, false
	m.cursor, m.offset = 0, 0
	m.searchID++
	if strings.TrimSpace(m.query.Value()) == "" {
		return nil
	}
	id := m.searchID
	return func() tea.Msg { return debounceMsg{id: id} }
}

func (m *model) leaveDeep() {
	m.stopSearch()
	m.searchID++
	m.deep = false
	m.query.Prompt = filterPrompt
	m.query.Placeholder = filterPlaceholder
	m.query.SetValue("")
	m.results, m.terms, m.searched = nil, nil, false
	m.cursor, m.offset = 0, 0
	m.refilter()
}

func (m *model) stopSearch() {
	if m.run != nil {
		m.run.cancel()
		m.run = nil
	}
}

// queryChangedDeep restarts the debounce after the query changed in Deep search.
// The previous results stay on screen until new ones arrive.
func (m *model) queryChangedDeep() tea.Cmd {
	m.stopSearch()
	m.searchID++
	if strings.TrimSpace(m.query.Value()) == "" {
		m.results, m.terms, m.searched = nil, nil, false
		m.cursor, m.offset = 0, 0
		return nil
	}
	id := m.searchID
	return tea.Tick(debounceDelay, func(time.Time) tea.Msg { return debounceMsg{id: id} })
}

func (m *model) startSearch(id int) tea.Cmd {
	q := strings.TrimSpace(m.query.Value())
	if id != m.searchID || !m.deep || q == "" || m.opts.Search == nil {
		return nil
	}
	m.stopSearch()
	ctx, cancel := context.WithCancel(context.Background())
	run := &searchRun{id: id, ctx: ctx, cancel: cancel}
	run.total.Store(int64(len(m.all)))
	m.run = run
	search := m.opts.Search
	do := func() tea.Msg {
		res, err := search(ctx, q, func(done, total int) {
			run.total.Store(int64(total))
			run.done.Store(int64(done))
		})
		return searchDoneMsg{id: id, results: res, err: err}
	}
	return tea.Batch(do, progressTick(id))
}

func progressTick(id int) tea.Cmd {
	return tea.Tick(progressEvery, func(time.Time) tea.Msg { return progressMsg{id: id} })
}

func (m *model) searchDone(msg searchDoneMsg) {
	if msg.id != m.searchID || !m.deep {
		return // stale: the query changed since this search started
	}
	m.stopSearch()
	if msg.err != nil {
		if !errors.Is(msg.err, context.Canceled) {
			m.setStatus("search failed: "+msg.err.Error(), false)
		}
		return
	}
	m.results, m.searched = msg.results, true
	// The id still matches, so the query is the one this search ran with.
	m.terms = claude.NewMatcher(m.query.Value()).Terms()
	m.cursor, m.offset = 0, 0
}

func (m *model) deepHeader() string {
	switch {
	case m.run != nil:
		return fmt.Sprintf(" · searching %d/%d", m.run.done.Load(), m.run.total.Load())
	case m.searched:
		return fmt.Sprintf(" · deep · %d match", len(m.results))
	}
	return " · deep"
}

func (m *model) deepEmpty() string {
	switch {
	case strings.TrimSpace(m.query.Value()) == "":
		return `  type to search inside transcripts  ·  "quotes" for a phrase`
	case !m.searched:
		return "  searching…"
	}
	return "  nothing matches"
}

// renderSnippets wraps each snippet to w, then highlights the search terms, so the
// styling never confuses the wrapping. At most avail lines are used.
func (m model) renderSnippets(snippets []string, w, avail int) string {
	wrap := lipgloss.NewStyle().Width(w)
	var blocks []string
	used := 0
	for _, sn := range snippets {
		lines := strings.Split(wrap.Render(sn), "\n")
		if len(lines) > 5 {
			lines = append(lines[:5], "…")
		}
		n := len(lines) + 1
		if used+n > avail && len(blocks) > 0 {
			break
		}
		for i, l := range lines {
			lines[i] = highlight(strings.TrimRight(l, " "), m.terms, renderHit)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
		used += n
	}
	return strings.Join(blocks, "\n\n")
}

// highlight wraps every case-insensitive occurrence of terms (already lowercased) in
// line with hl. Offsets are mapped rune by rune, so lowercasing that changes a rune's
// byte length does not shift the highlight.
func highlight(line string, terms []string, hl func(string) string) string {
	if len(terms) == 0 || line == "" {
		return line
	}
	var low strings.Builder
	origAt := make([]int, 0, len(line)+1) // byte in low -> start of its rune in line
	for i, r := range line {
		lr := unicode.ToLower(r)
		n := utf8.RuneLen(lr)
		low.WriteRune(lr)
		for k := 0; k < n; k++ {
			origAt = append(origAt, i)
		}
	}
	origAt = append(origAt, len(line))
	ls := low.String()

	mark := make([]bool, len(line))
	found := false
	for _, t := range terms {
		if t == "" {
			continue
		}
		for from := 0; ; {
			j := strings.Index(ls[from:], t)
			if j < 0 {
				break
			}
			a, b := from+j, from+j+len(t)
			for k := origAt[a]; k < origAt[b]; k++ {
				mark[k] = true
				found = true
			}
			from = b
		}
	}
	if !found {
		return line
	}
	var out strings.Builder
	for i := 0; i < len(line); {
		j := i
		for j < len(line) && mark[j] == mark[i] {
			j++
		}
		if mark[i] {
			out.WriteString(hl(line[i:j]))
		} else {
			out.WriteString(line[i:j])
		}
		i = j
	}
	return out.String()
}
