package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/hamzafer/unpause/internal/session"
)

// SnippetLimit is how many snippets a search keeps per Session.
const SnippetLimit = 3

// snippetContext is how many characters a snippet shows on each side of a match.
const snippetContext = 90

// term is one lowercased search term and its JSON-escaped form.
type term struct {
	text string
	// raw is the term as it appears inside a JSON string, used to skip lines before
	// decoding them. JSON escaping (backslashes, quotes) is applied so such terms still match.
	raw []byte
}

// Matcher is a Deep search query: case-insensitive substring terms, each of which
// must appear somewhere in a Session's talk or Title.
type Matcher struct {
	terms []term
	ascii bool
}

// NewMatcher parses q into terms: split on whitespace, "double quotes" make one
// phrase term, an unclosed quote runs to the end. Repeated terms count once.
func NewMatcher(q string) *Matcher {
	m := &Matcher{ascii: true}
	for _, t := range parseTerms(strings.ToLower(q)) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(t)
		esc := bytes.TrimSpace(buf.Bytes())
		m.terms = append(m.terms, term{text: t, raw: esc[1 : len(esc)-1]})
		for i := 0; i < len(t); i++ {
			if t[i] >= utf8.RuneSelf {
				m.ascii = false
			}
		}
	}
	return m
}

func parseTerms(q string) []string {
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			add(cur.String())
			cur.Reset()
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			add(cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	add(cur.String())
	return out
}

// Terms returns the parsed, lowercased terms, for highlighting.
func (m *Matcher) Terms() []string {
	var out []string
	for _, t := range m.terms {
		out = append(out, t.text)
	}
	return out
}

// SearchResult is what one Session yielded.
type SearchResult struct {
	Hits     int      // total occurrences of all terms
	Snippets []string // up to SnippetLimit, in file order
	Seen     []bool   // per term, in Terms order
}

// All reports whether every term was seen. A query with no terms matches nothing.
func (r *SearchResult) All() bool {
	if len(r.Seen) == 0 {
		return false
	}
	for _, s := range r.Seen {
		if !s {
			return false
		}
	}
	return true
}

func (m *Matcher) newResult() *SearchResult {
	return &SearchResult{Seen: make([]bool, len(m.terms))}
}

// MatchTitle adds the terms found in a Session's Title to r. Title matches give no snippet.
func (m *Matcher) MatchTitle(title string, r *SearchResult) {
	lt := strings.ToLower(title)
	for i, t := range m.terms {
		if n := strings.Count(lt, t.text); n > 0 {
			r.Hits += n
			r.Seen[i] = true
		}
	}
}

// SearchFile scans one transcript's talk: the text blocks of user and assistant
// records. Tool calls, tool results and thinking are not searched.
func (m *Matcher) SearchFile(ctx context.Context, path string) (*SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res := m.newResult()
	if len(m.terms) == 0 {
		return res, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lower []byte
	n := 0
	err = eachLine(f, func(raw []byte) error {
		n++
		if n%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if !isMessageLine(raw) || isMeta(raw) {
			return nil
		}
		// The pre-filter only knows each term's canonical JSON escaping. A line with
		// \u or \/ escapes may spell a term differently, so it is always decoded.
		lower = lowerInto(lower[:0], raw, m.ascii)
		if !m.anyRaw(lower) && !bytes.Contains(raw, []byte(`\u`)) && !bytes.Contains(raw, []byte(`\/`)) {
			return nil
		}
		_, _, blocks, ok := decodeMessage(raw)
		if !ok {
			return nil
		}
		for _, b := range blocks {
			if b.Type != "text" {
				continue
			}
			if t, ok := talk(b.Text); ok {
				m.match(t, res)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// anyRaw is the per-line pre-filter: does the lowercased raw line hold any term?
func (m *Matcher) anyRaw(lower []byte) bool {
	for _, t := range m.terms {
		if bytes.Contains(lower, t.raw) {
			return true
		}
	}
	return false
}

// lowerInto lowercases src into dst. For ASCII terms, ASCII folding is enough:
// non-ASCII bytes can't be part of a match.
func lowerInto(dst, src []byte, ascii bool) []byte {
	if !ascii {
		return append(dst, bytes.ToLower(src)...)
	}
	for _, c := range src {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

// match counts every term in text and keeps snippets around matches, in text order.
// A match that falls inside the previous snippet's window adds no snippet of its own.
func (m *Matcher) match(text string, r *SearchResult) {
	lt := strings.ToLower(text)
	src := text
	if len(lt) != len(text) {
		src = lt // byte offsets only line up with the original when lowercasing kept lengths
	}
	type hit struct{ start, end int }
	var hits []hit
	for i, t := range m.terms {
		from := 0
		for {
			j := strings.Index(lt[from:], t.text)
			if j < 0 {
				break
			}
			at := from + j
			r.Hits++
			r.Seen[i] = true
			hits = append(hits, hit{at, at + len(t.text)})
			from = at + len(t.text)
		}
	}
	if len(r.Snippets) >= SnippetLimit || len(hits) == 0 {
		return
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].start < hits[b].start })
	covered := -1
	for _, h := range hits {
		if len(r.Snippets) >= SnippetLimit {
			return
		}
		if h.start < covered {
			continue
		}
		s, end := snippet(src, h.start, h.end)
		r.Snippets = append(r.Snippets, s)
		covered = end
	}
}

// snippet returns about snippetContext characters on each side of src[start:end], on
// one line, and the byte offset where its window ends.
func snippet(src string, start, end int) (string, int) {
	a := start
	for n := 0; a > 0 && n < snippetContext; n++ {
		_, size := utf8.DecodeLastRuneInString(src[:a])
		a -= size
	}
	b := end
	for n := 0; b < len(src) && n < snippetContext; n++ {
		_, size := utf8.DecodeRuneInString(src[b:])
		b += size
	}
	s := strings.Join(strings.Fields(stripControl(src[a:b])), " ")
	if a > 0 {
		s = "…" + s
	}
	if b < len(src) {
		s += "…"
	}
	return s, b
}

// notTalk are the tags Claude Code wraps around text it stores as a user message
// although nobody typed it: background task reports, command and shell output,
// reminders and fork instructions.
var notTalk = []string{
	"<task-notification>", "<local-command-", "<bash-stdout>", "<bash-stderr>",
	"<system-reminder>", "<fork-boilerplate>",
}

// talk returns the part of a text block that was actually said. Text that isn't talk
// gives ok false. A slash command keeps its name and arguments without the wrapper
// tags (`/goal find padel`), a shell command its input (`! ls`).
func talk(text string) (string, bool) {
	t := strings.TrimSpace(text)
	for _, tag := range notTalk {
		if strings.HasPrefix(t, tag) {
			return "", false
		}
	}
	if name, ok := between(t, "<command-name>", "</command-name>"); ok {
		args, _ := between(t, "<command-args>", "</command-args>")
		return strings.TrimSpace(name + " " + args), true
	}
	if in, ok := between(t, "<bash-input>", "</bash-input>"); ok && strings.HasPrefix(t, "<bash-input>") {
		return "! " + strings.TrimSpace(in), true
	}
	return text, true
}

// between returns the text between the first open tag and the close tag after it.
func between(s, open, close string) (string, bool) {
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// isMeta reports whether a record is marked isMeta at the top level: text Claude Code
// injected as a user message (reminders, skill bodies), not something anyone typed.
func isMeta(raw []byte) bool {
	if !bytes.Contains(raw, []byte(`"isMeta"`)) {
		return false
	}
	var r struct {
		IsMeta bool `json:"isMeta"`
	}
	return json.Unmarshal(raw, &r) == nil && r.IsMeta
}

// stripControl turns control characters (terminal escapes from pasted output, for
// example) into spaces, so a snippet can't drive the terminal it is printed to.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// Result is one Session a Deep search found.
type Result struct {
	Session  *session.Session `json:"session"`
	Hits     int              `json:"hits"`
	Snippets []string         `json:"snippets"`
}

// Search runs a Deep search over sessions: every transcript's talk is scanned in
// parallel, the Title counts when it is a Name, Desktop title or AI title, and only
// Sessions where every term matched are kept. Results are ranked by hits, then by
// last activity, newest first. A transcript that can't be read just doesn't match.
//
// progress, if not nil, is called after each transcript with how many are done out of
// how many in total. Calls are serialized, never concurrent, and done only grows.
// On cancellation Search returns ctx.Err().
func Search(ctx context.Context, sessions []*session.Session, query string, progress func(done, total int)) ([]Result, error) {
	m := NewMatcher(query)
	if len(m.terms) == 0 {
		return nil, ctx.Err()
	}
	results := make([]*SearchResult, len(sessions))
	var mu sync.Mutex
	done := 0

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				s := sessions[i]
				r, err := m.SearchFile(ctx, s.Path)
				if err != nil {
					r = m.newResult()
				}
				if s.CustomTitle != "" || s.DesktopTitle != "" || s.AITitle != "" {
					// A Title made from the first prompt would count that prompt twice.
					m.MatchTitle(s.Title(), r)
				}
				results[i] = r
				if progress != nil {
					mu.Lock()
					done++
					progress(done, len(sessions))
					mu.Unlock()
				}
			}
		}()
	}
feed:
	for i := range sessions {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var out []Result
	for i, r := range results {
		if !r.All() {
			continue
		}
		snips := r.Snippets
		if snips == nil {
			snips = []string{}
		}
		out = append(out, Result{Session: sessions[i], Hits: r.Hits, Snippets: snips})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Hits != out[b].Hits {
			return out[a].Hits > out[b].Hits
		}
		return out[a].Session.LastActive.After(out[b].Session.LastActive)
	})
	return out, nil
}
