package mcpserver

import (
	"bufio"
	"bytes"
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
	"unicode/utf8"

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

// inputError is a call the handler turns away for what the client sent, such as an
// empty query. Its text goes to the client; the log only says "invalid input".
type inputError string

func (e inputError) Error() string { return string(e) }

// The error classes an Entry can carry. The log never holds an error's text: it can
// name paths on this machine.
const (
	errClassNotFound = "not found"
	errClassInput    = "invalid input"
	errClassCanceled = "canceled"
	errClassFailed   = "failed"
)

// errorClass maps a tool error to one of the fixed classes.
func errorClass(err error) string {
	var in inputError
	switch {
	case errors.Is(err, errNotFound):
		return errClassNotFound
	case errors.As(err, &in):
		return errClassInput
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errClassCanceled
	default:
		return errClassFailed
	}
}

// maxField is the most bytes any one string the client sent takes up in the log.
const maxField = 500

// clipString cuts s to at most maxField bytes on a UTF-8 boundary, ending in "…".
func clipString(s string) string {
	if len(s) <= maxField {
		return s
	}
	const ell = "…"
	n := maxField - len(ell)
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + ell
}

// clipInput re-encodes a call's input with every string in it clipped.
func clipInput(raw []byte) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return json.RawMessage(`{}`)
	}
	out, err := json.Marshal(clipValue(v))
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return out
}

func clipValue(v any) any {
	switch x := v.(type) {
	case string:
		return clipString(x)
	case []any:
		for i := range x {
			x[i] = clipValue(x[i])
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[clipString(k)] = clipValue(val)
		}
		return out
	}
	return v
}

// newEntry starts an Entry for a call to tool from the client behind req.
func newEntry(start time.Time, tool string, req *mcp.CallToolRequest) Entry {
	e := Entry{At: start.UTC().Format(time.RFC3339), Tool: clipString(tool), Input: json.RawMessage(`{}`), SessionIDs: []string{}}
	if req != nil {
		if ci := req.ClientInfo(); ci != nil {
			e.Client, e.ClientVersion = clipString(ci.Name), clipString(ci.Version)
		}
	}
	return e
}

// handledKey marks, in a tools/call context, that a tool handler ran and logged the call.
type handledKey struct{}

// addTool registers a tool with its call wrapped in the Access log, so no tool can skip
// it. returned names the Sessions in a result, how many items came back, and the total.
func addTool[In, Out any](s *mcp.Server, rec Recorder, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out], returned func(Out) (ids []string, count, total int)) {
	mcp.AddTool(s, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		start := time.Now()
		res, out, err := h(ctx, req, in)
		if rec == nil {
			return res, out, err
		}
		if handled, ok := ctx.Value(handledKey{}).(*bool); ok {
			*handled = true
		}
		e := newEntry(start, t.Name, req)
		if raw, merr := json.Marshal(in); merr == nil {
			e.Input = clipInput(raw)
		}
		if err != nil {
			e.Error = errorClass(err)
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

// logRejected records the tools/call requests the SDK turns away before any handler
// runs (an unknown tool, arguments of the wrong type) as invalid input. Calls a handler
// saw are logged there, so nothing is logged twice.
func logRejected(rec Recorder) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			start := time.Now()
			handled := new(bool)
			res, err := next(context.WithValue(ctx, handledKey{}, handled), method, req)
			if !*handled {
				ctr, _ := req.(*mcp.CallToolRequest)
				var tool string
				if ctr != nil && ctr.Params != nil {
					tool = ctr.Params.Name
				}
				e := newEntry(start, tool, ctr)
				e.Error = errClassInput
				e.DurationMS = time.Since(start).Milliseconds()
				rec(e)
			}
			return res, err
		}
	}
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
	// Another unpause mcp may be writing too: the lock keeps two of them from both
	// rotating a full log, which would replace access.jsonl.1 with a nearly empty file.
	unlock, err := lockFile(l.Path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	if fi, err := os.Lstat(l.Path); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file, refusing to write", l.Path)
		}
		if fi.Size() >= l.MaxSize {
			if err := os.Rename(l.Path, l.Path+".1"); err != nil {
				return err
			}
		}
	}
	f, err := openNoFollow(l.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return fmt.Errorf("%s is not a regular file, refusing to write", l.Path)
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

// maxLine is the longest line ReadLog parses; longer ones are skipped. A line the server
// writes stays far below it, since every string in it is clipped.
const maxLine = 1 << 20

// ReadLog reads the rotated file and then the current one, oldest first. Missing files
// are no Entries; lines that don't parse, or are over maxLine, are skipped.
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
		err = eachLine(f, func(line []byte) {
			var e Entry
			if json.Unmarshal(line, &e) == nil {
				out = append(out, e)
			}
		})
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// eachLine calls fn with every line of r up to maxLine bytes, skipping longer ones
// without holding them in memory.
func eachLine(r io.Reader, fn func([]byte)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	var buf []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > maxLine {
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if !tooLong && len(bytes.TrimSpace(buf)) > 0 {
			fn(buf)
		}
		buf, tooLong = buf[:0], false
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
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
