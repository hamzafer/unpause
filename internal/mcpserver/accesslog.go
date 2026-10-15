package mcpserver

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Entry is one line of the Access log: one tool call. It holds what the client sent and
// which Sessions came back, never message text, titles, snippets or folders (ADR 0013).
type Entry struct {
	At            string          `json:"at"`
	Client        string          `json:"client"`
	ClientVersion string          `json:"client_version"`
	Tool          string          `json:"tool"`
	Input         json.RawMessage `json:"input"`
	SessionIDs    []string        `json:"session_ids"`
	Count         int             `json:"count"`
	Total         int             `json:"total"`
	DurationMS    int64           `json:"duration_ms"`
	Error         string          `json:"error,omitempty"`
}

// Recorder receives one Entry per tool call. A nil Recorder records nothing.
type Recorder func(Entry)

// errNotFound is what get_session fails with for an unknown or Excluded id.
var errNotFound = errors.New("not found")

// addTool registers a tool with its call wrapped in the Access log, so no tool can skip
// it. returned names the Sessions in a result, how many items came back, and the total.
func addTool[In, Out any](s *mcp.Server, rec Recorder, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out], returned func(Out) (ids []string, count, total int)) {
	mcp.AddTool(s, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		start := time.Now()
		res, out, err := h(ctx, req, in)
		if rec == nil {
			return res, out, err
		}
		e := Entry{At: start.UTC().Format(time.RFC3339), Tool: t.Name, SessionIDs: []string{}}
		if req != nil {
			if ci := req.ClientInfo(); ci != nil {
				e.Client, e.ClientVersion = ci.Name, ci.Version
			}
		}
		e.Input, _ = json.Marshal(in)
		if err != nil {
			e.Error = err.Error()
			if errors.Is(err, errNotFound) {
				e.Error = errNotFound.Error()
			}
		} else {
			ids, count, total := returned(out)
			e.SessionIDs = append(e.SessionIDs, ids...)
			e.Count, e.Total = count, total
		}
		e.DurationMS = time.Since(start).Milliseconds()
		rec(e)
		return res, out, err
	})
}

// LogPath is the Access log next to the index cache at cachePath.
func LogPath(cachePath string) string {
	return filepath.Join(filepath.Dir(cachePath), "access.jsonl")
}

// AccessLog appends Entries to a JSON-lines file, rotating it to Path+".1" once it
// reaches MaxSize. The file is 0600: it shows what was read.
type AccessLog struct {
	Path    string
	MaxSize int64
	Stderr  io.Writer // where a failed write is reported

	mu sync.Mutex
}

// NewAccessLog logs to path, rotating at 10 MB.
func NewAccessLog(path string) *AccessLog {
	return &AccessLog{Path: path, MaxSize: 10 << 20, Stderr: os.Stderr}
}

// Record appends e as one line. A failure goes to Stderr; it never fails the tool call.
func (l *AccessLog) Record(e Entry) {
	if err := l.append(e); err != nil && l.Stderr != nil {
		fmt.Fprintf(l.Stderr, "unpause: access log: %v\n", err)
	}
}

func (l *AccessLog) append(e Entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	if fi, err := os.Stat(l.Path); err == nil && fi.Size() >= l.MaxSize {
		if err := os.Rename(l.Path, l.Path+".1"); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadLog reads the rotated file and then the current one, oldest first. Missing files
// are no Entries; lines that don't parse are skipped.
func ReadLog(path string) ([]Entry, error) {
	var out []Entry
	for _, p := range []string{path + ".1", path} {
		f, err := os.Open(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				out = append(out, e)
			}
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Tally is a name and how often it came up.
type Tally struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Stats summarizes the Access log for `unpause mcp stats`.
type Stats struct {
	Calls      int       `json:"calls"`
	Errors     int       `json:"errors"`
	First      time.Time `json:"first,omitzero"`
	Last       time.Time `json:"last,omitzero"`
	Tools      []Tally   `json:"tools"`
	Clients    []Tally   `json:"clients"`
	MostRead   []Tally   `json:"most_read"` // Session ids by successful get_session calls
	TopQueries []Tally   `json:"top_queries"`
}

const topN = 10

// Summarize counts the Entries at or after since (all of them when since is zero).
// Entries whose time doesn't parse are skipped.
func Summarize(entries []Entry, since time.Time) Stats {
	tools, clients, reads, queries := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var st Stats
	for _, e := range entries {
		at, err := time.Parse(time.RFC3339, e.At)
		if err != nil || at.Before(since) {
			continue
		}
		st.Calls++
		if st.First.IsZero() || at.Before(st.First) {
			st.First = at
		}
		if at.After(st.Last) {
			st.Last = at
		}
		tools[e.Tool]++
		client := e.Client
		if client == "" {
			client = "(unknown)"
		}
		clients[client]++
		if e.Error != "" {
			st.Errors++
		}
		switch e.Tool {
		case "get_session":
			if e.Error == "" {
				for _, id := range e.SessionIDs {
					reads[id]++
				}
			}
		case "search_sessions":
			var in struct {
				Query string `json:"query"`
			}
			if json.Unmarshal(e.Input, &in) == nil && strings.TrimSpace(in.Query) != "" {
				queries[strings.TrimSpace(in.Query)]++
			}
		}
	}
	st.Tools = tallies(tools, 0)
	st.Clients = tallies(clients, 0)
	st.MostRead = tallies(reads, topN)
	st.TopQueries = tallies(queries, topN)
	return st
}

// tallies sorts m by count, then name, keeping at most n (all when n is 0).
func tallies(m map[string]int, n int) []Tally {
	out := []Tally{}
	for k, v := range m {
		out = append(out, Tally{k, v})
	}
	slices.SortFunc(out, func(a, b Tally) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Name, b.Name))
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
