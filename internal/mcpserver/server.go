// Package mcpserver is `unpause mcp`: a read-only MCP server that lets another assistant
// list, search and read Sessions. It never writes to a Transcript.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/session"
)

// Loader returns the Sessions to serve, newest first and already default-filtered
// (non-empty, not sidechain), plus the [mcp] exclude globs. It runs on every tool call.
type Loader func(ctx context.Context) (sessions []*session.Session, exclude []string, err error)

// Limits from the spec.
const (
	listDefault, listMax     = 20, 100
	searchDefault, searchMax = 10, 50
	getDefault, getMax       = 50, 200
	maxCharsDefault          = 20000
)

// Summary is how a Session is described in every tool's output.
type Summary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Account    string `json:"account"`
	Project    string `json:"project" jsonschema:"repo folder name"`
	CWD        string `json:"cwd"`
	Branch     string `json:"branch,omitempty"`
	Created    string `json:"created,omitempty"`
	LastActive string `json:"last_active,omitempty"`
	Messages   int    `json:"messages" jsonschema:"user prompts plus assistant replies"`
	Live       string `json:"live,omitempty" jsonschema:"busy or idle when the session is running now"`
	Cowork     bool   `json:"cowork,omitempty" jsonschema:"a Claude Desktop Cowork session"`
}

// ListInput is list_sessions' arguments.
type ListInput struct {
	Account string `json:"account,omitempty" jsonschema:"only this Account label, exact (e.g. personal, work)"`
	Project string `json:"project,omitempty" jsonschema:"case-insensitive substring of the working directory"`
	Live    *bool  `json:"live,omitempty" jsonschema:"true: only sessions running now; false: only ones that are not"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size, default 20, max 100"`
	Offset  int    `json:"offset,omitempty" jsonschema:"how many to skip, default 0"`
}

// ListOutput is list_sessions' result.
type ListOutput struct {
	Total      int       `json:"total"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty" jsonschema:"offset of the next page; absent on the last page"`
	Sessions   []Summary `json:"sessions"`
}

// SearchInput is search_sessions' arguments.
type SearchInput struct {
	Query   string `json:"query" jsonschema:"terms separated by spaces, all of which must appear (case-insensitive substrings); wrap words in double quotes for an exact phrase"`
	Account string `json:"account,omitempty" jsonschema:"only this Account label, exact"`
	Project string `json:"project,omitempty" jsonschema:"case-insensitive substring of the working directory"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size, default 10, max 50"`
	Offset  int    `json:"offset,omitempty" jsonschema:"how many to skip, default 0"`
}

// SearchHit is one matching Session.
type SearchHit struct {
	Summary
	Hits     int      `json:"hits" jsonschema:"total matches of all terms"`
	Snippets []string `json:"snippets" jsonschema:"up to 3 excerpts around a match"`
}

// SearchOutput is search_sessions' result.
type SearchOutput struct {
	Total      int         `json:"total"`
	Offset     int         `json:"offset"`
	NextOffset *int        `json:"next_offset,omitempty"`
	Results    []SearchHit `json:"results"`
}

// GetInput is get_session's arguments.
type GetInput struct {
	ID       string `json:"id" jsonschema:"session id, id prefix, or Name (same lookup as unpause open)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"index of the first message, default 0"`
	Limit    int    `json:"limit,omitempty" jsonschema:"messages per page, default 50, max 200"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"longest content block kept whole, default 20000; longer ones are cut"`
}

// GetOutput is get_session's result.
type GetOutput struct {
	Session    Summary              `json:"session"`
	Total      int                  `json:"total"`
	Offset     int                  `json:"offset"`
	NextOffset *int                 `json:"next_offset,omitempty"`
	Messages   []claude.FullMessage `json:"messages"`
}

// Run serves over stdin and stdout until the client disconnects or ctx ends.
func Run(ctx context.Context, version string, load Loader) error {
	return New(version, load).Run(ctx, &mcp.StdioTransport{})
}

// New builds the server with its three read-only tools.
func New(version string, load Loader) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "unpause", Version: version}, &mcp.ServerOptions{
		Instructions: "Read-only access to the user's local Claude Code sessions. " +
			"Use list_sessions to browse, search_sessions to find text, get_session to read a transcript page by page.",
	})
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	h := &handlers{load: load}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List Claude Code sessions on this machine, newest first, with title, account, project, working directory, branch, times, message count and live state.",
		Annotations: ro,
	}, h.list)
	mcp.AddTool(s, &mcp.Tool{
		Name: "search_sessions",
		Description: "Find sessions by what was said in them. Searches the talk only: the text of user prompts and assistant replies, plus the title " +
			"when it is a name, Claude Desktop title or AI title. Tool calls, tool results and thinking are not searched. " +
			"The query is split into terms on whitespace, and \"double quotes\" make one phrase; each term is a case-insensitive substring, " +
			"and a session matches only when every term appears somewhere in it, not necessarily in the same message. " +
			"Results are ranked by total matches, ties newest first, with a match count and up to 3 snippets.",
		Annotations: ro,
	}, h.search)
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_session",
		Description: "Read one session's transcript in file order, page by page. Each message has blocks: text, thinking, tool_use (name and input) " +
			"and tool_result (text, is_error). Blocks longer than max_chars are cut with a [… N more chars] marker.",
		Annotations: ro,
	}, h.get)
	return s
}

type handlers struct {
	load Loader
}

// visible loads the Sessions and drops the Excluded ones.
func (h *handlers) visible(ctx context.Context) ([]*session.Session, error) {
	all, exclude, err := h.load(ctx)
	if err != nil {
		return nil, err
	}
	if len(exclude) == 0 {
		return all, nil
	}
	x := newExcluder(exclude)
	out := all[:0:0]
	for _, s := range all {
		if !x.hides(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (h *handlers) list(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, ListOutput, error) {
	ss, err := h.visible(ctx)
	if err != nil {
		return nil, ListOutput{}, err
	}
	var match []*session.Session
	for _, s := range ss {
		if !keep(s, in.Account, in.Project) {
			continue
		}
		if in.Live != nil && (s.Live != nil) != *in.Live {
			continue
		}
		match = append(match, s)
	}
	limit := clamp(in.Limit, listDefault, listMax)
	offset := max(in.Offset, 0)
	page, next := pageOf(len(match), offset, limit)
	out := ListOutput{Total: len(match), Offset: offset, NextOffset: next, Sessions: []Summary{}}
	for _, s := range match[page[0]:page[1]] {
		out.Sessions = append(out.Sessions, summarize(s))
	}
	return nil, out, nil
}

func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, SearchOutput{}, errors.New("query is empty")
	}
	ss, err := h.visible(ctx)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	var cands []*session.Session
	for _, s := range ss {
		if keep(s, in.Account, in.Project) {
			cands = append(cands, s)
		}
	}
	results, err := claude.Search(ctx, cands, in.Query, nil)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	hits := make([]SearchHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, SearchHit{Summary: summarize(r.Session), Hits: r.Hits, Snippets: r.Snippets})
	}
	limit := clamp(in.Limit, searchDefault, searchMax)
	offset := max(in.Offset, 0)
	page, next := pageOf(len(hits), offset, limit)
	out := SearchOutput{Total: len(hits), Offset: offset, NextOffset: next, Results: []SearchHit{}}
	out.Results = append(out.Results, hits[page[0]:page[1]]...)
	return nil, out, nil
}

func (h *handlers) get(ctx context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, GetOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return nil, GetOutput{}, errors.New("id is empty")
	}
	ss, err := h.visible(ctx)
	if err != nil {
		return nil, GetOutput{}, err
	}
	s := session.Find(ss, in.ID)
	if s == nil {
		return nil, GetOutput{}, fmt.Errorf("session %q not found", in.ID)
	}
	limit := clamp(in.Limit, getDefault, getMax)
	offset := max(in.Offset, 0)
	maxChars := in.MaxChars
	if maxChars <= 0 {
		maxChars = maxCharsDefault
	}
	msgs, total, err := claude.ReadMessages(s.Path, offset, limit, maxChars)
	if err != nil {
		return nil, GetOutput{}, err
	}
	_, next := pageOf(total, offset, limit)
	return nil, GetOutput{Session: summarize(s), Total: total, Offset: offset, NextOffset: next, Messages: msgs}, nil
}

// keep applies the account and project filters shared by list and search.
func keep(s *session.Session, account, project string) bool {
	if account != "" && s.Account != account {
		return false
	}
	if project != "" && !strings.Contains(strings.ToLower(s.CWD), strings.ToLower(project)) {
		return false
	}
	return true
}

func summarize(s *session.Session) Summary {
	out := Summary{
		ID: s.ID, Title: s.Title(), Account: s.Account, CWD: s.CWD, Branch: s.Branch,
		Created: stamp(s.Created), LastActive: stamp(s.LastActive), Messages: s.Messages, Cowork: s.Cowork,
	}
	if s.CWD != "" {
		out.Project = s.Repo()
	}
	if s.Live != nil {
		out.Live = s.Live.Status
		if out.Live == "" {
			out.Live = "idle"
		}
	}
	return out
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func clamp(n, def, hi int) int {
	switch {
	case n <= 0:
		return def
	case n > hi:
		return hi
	}
	return n
}

// pageOf returns the [start, end) slice bounds for one page and the next page's offset, if any.
func pageOf(total, offset, limit int) ([2]int, *int) {
	start := min(offset, total)
	end := min(start+limit, total)
	if end < total {
		n := end
		return [2]int{start, end}, &n
	}
	return [2]int{start, end}, nil
}

// foldCase makes exclude matching case-insensitive. macOS file systems are, by default.
var foldCase = runtime.GOOS == "darwin"

// excluder decides which Sessions the [mcp] exclude globs hide. It is built per tool
// call and caches symlink lookups for that call.
type excluder struct {
	patterns []string
	resolved map[string]string // dir -> its symlink-free path, "" when it can't be resolved
}

func newExcluder(patterns []string) *excluder {
	x := &excluder{resolved: map[string]string{}}
	for _, p := range patterns {
		for _, q := range withRealPrefix(p) {
			if foldCase {
				q = strings.ToLower(q)
			}
			x.patterns = append(x.patterns, q)
		}
	}
	return x
}

// withRealPrefix returns p, plus p with symlinks resolved in its literal leading folders
// (the ones before the first glob character), so a pattern written through a symlink
// still matches the real folder.
func withRealPrefix(p string) []string {
	if !strings.HasPrefix(p, "/") {
		return []string{p}
	}
	parts := strings.Split(p, "/")
	n := len(parts)
	for i, part := range parts {
		if strings.ContainsAny(part, `*?[\`) {
			n = i
			break
		}
	}
	prefix := strings.Join(parts[:n], "/")
	real, err := filepath.EvalSymlinks(prefix)
	if prefix == "" || err != nil || real == prefix {
		return []string{p}
	}
	return []string{p, real + strings.TrimPrefix(p, prefix)}
}

// hides reports whether s is Excluded: any folder it worked in (every cwd, plus a
// Cowork Session's selected folders) matches a pattern, as is or with symlinks
// resolved. When any pattern is set, a Session with no folder at all is hidden too.
func (x *excluder) hides(s *session.Session) bool {
	if len(x.patterns) == 0 {
		return false
	}
	dirs := s.Dirs
	if s.CWD != "" && !slices.Contains(dirs, s.CWD) {
		dirs = append(slices.Clip(dirs), s.CWD)
	}
	if len(dirs) == 0 {
		return true
	}
	for _, d := range dirs {
		if x.matches(d) {
			return true
		}
		if r := x.resolve(d); r != "" && r != d && x.matches(r) {
			return true
		}
	}
	return false
}

func (x *excluder) resolve(dir string) string {
	r, ok := x.resolved[dir]
	if !ok {
		r, _ = filepath.EvalSymlinks(dir)
		x.resolved[dir] = r
	}
	return r
}

// matches reports whether dir, or a folder above it, matches a pattern (path.Match syntax).
// A pattern that does not start with "/" may match the trailing folders of a path, so
// "*/secret-repo" matches /anything/secret-repo.
func (x *excluder) matches(dir string) bool {
	if foldCase {
		dir = strings.ToLower(dir)
	}
	for p := path.Clean(dir); ; p = path.Dir(p) {
		for _, pat := range x.patterns {
			if matchPath(pat, p) {
				return true
			}
		}
		if p == "/" || p == "." {
			return false
		}
	}
}

func matchPath(pat, p string) bool {
	if ok, _ := path.Match(pat, p); ok {
		return true
	}
	if strings.HasPrefix(pat, "/") {
		return false
	}
	// Relative pattern: try each tail of p made of whole folders.
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if ok, _ := path.Match(pat, p[i+1:]); ok {
				return true
			}
		}
	}
	return false
}
