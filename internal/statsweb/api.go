// Package statsweb serves `unpause mcp stats --web`: the stats page from the embedded
// export and a small JSON API over the Access log (ADR 0014). The API is built from the
// log's Entries plus a title lookup in the local index, so it is tested without a network.
package statsweb

import (
	"cmp"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/hamzafer/unpause/internal/mcpserver"
)

// Info is what the local index says about a Session. The log never holds it (ADR 0013).
type Info struct {
	Title   string
	Account string
	Project string
}

// Lookup finds a Session in the local index by its full id.
type Lookup func(id string) (Info, bool)

// ErrRange is a range the API doesn't know.
var ErrRange = errors.New("range must be one of 24h, 7d, 30d, all")

// spans are the ranges the page offers; "all" has no start.
var spans = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"all": 0,
}

// Window is the KPI row for one time window.
type Window struct {
	Calls        int    `json:"calls"`
	SessionsRead int    `json:"sessions_read"` // distinct Sessions from successful get_session calls
	Clients      int    `json:"clients"`
	Errors       int    `json:"errors"`
	P50MS        *int64 `json:"p50_ms"` // lower median duration of successful calls; null when none
}

// KPIs is the range's Window plus the same-length window just before it (null for "all").
type KPIs struct {
	Window
	Prev *Window `json:"prev"`
}

// Client is one MCP client with calls in the range.
type Client struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Slot is the client's color, 0 to 4, by first appearance in the whole log, so it
	// doesn't change with the range. -1 means the sixth client or later: draw it as "Other".
	Slot  int `json:"slot"`
	Calls int `json:"calls"`
	// SessionsRead counts the distinct Sessions this client read with a successful get_session.
	SessionsRead int    `json:"sessions_read"`
	LastSeen     string `json:"last_seen"`
}

// OtherSlot is the Slot of every client past the fifth.
const OtherSlot = -1

const maxSlots = 5

// Point is one bucket of the calls-over-time series. T is the bucket's start in UTC; the
// bucket boundaries are local hours or local midnights of the machine running unpause.
type Point struct {
	T        string         `json:"t"`
	Covered  bool           `json:"covered"` // false before the first logged call: a gap, not a zero
	ByClient map[string]int `json:"by_client"`
}

// Series is calls over time, by client id.
type Series struct {
	Bucket string  `json:"bucket"` // "hour" or "day"
	Points []Point `json:"points"`
}

// Read is one of the Sessions read most through get_session.
type Read struct {
	ID       string `json:"id"`
	Title    string `json:"title"` // the short id when the Session is no longer indexed
	Account  string `json:"account"`
	Project  string `json:"project"`
	Reads    int    `json:"reads"`
	LastRead string `json:"last_read"`
}

// Query is a search_sessions query and how often it was sent.
type Query struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// Tool is a tool and how often it was called.
type Tool struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Call is one row of the recent calls: the Entry as logged plus the client's friendly name.
type Call struct {
	At         string          `json:"at"`
	Client     string          `json:"client"`
	ClientName string          `json:"client_name"`
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	SessionIDs []string        `json:"session_ids"`
	Count      int             `json:"count"`
	Total      int             `json:"total"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error"`
}

// Stats is the body of GET /api/stats.
type Stats struct {
	Range       string   `json:"range"`
	GeneratedAt string   `json:"generated_at"`
	Empty       bool     `json:"empty"` // no Entries in the log at all
	KPIs        KPIs     `json:"kpis"`
	Clients     []Client `json:"clients"`
	Series      Series   `json:"series"`
	MostRead    []Read   `json:"most_read"`
	TopQueries  []Query  `json:"top_queries"`
	Tools       []Tool   `json:"tools"`
	Recent      []Call   `json:"recent"`
}

const (
	topN      = 10
	recentN   = 50
	unknownID = "(unknown)"
)

// entry is an Entry whose time parsed, with its client id filled in.
type entry struct {
	at     time.Time
	client string
	*mcpserver.Entry
}

// earliest is the oldest time an Entry can sensibly have: the access log is from 2026, so
// anything before 2020 is a broken clock (or a zero time), not a call.
var earliest = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// maxAllSpan is how far back "all" reaches at most, so a long log can't build a huge series.
const maxAllSpan = 400 * 24 * time.Hour

// parse keeps the Entries whose time parses and is sane at now (from 2020 to a day ahead
// of now), oldest first (log order on ties). The rest count nowhere: not in a window, the
// series, the clients or their slots.
func parse(es []mcpserver.Entry, now time.Time) []entry {
	out := make([]entry, 0, len(es))
	latest := now.Add(24 * time.Hour)
	for i := range es {
		at, err := time.Parse(time.RFC3339, es[i].At)
		if err != nil || at.Before(earliest) || at.After(latest) {
			continue
		}
		c := es[i].Client
		if c == "" {
			c = unknownID
		}
		out = append(out, entry{at, c, &es[i]})
	}
	slices.SortStableFunc(out, func(a, b entry) int { return a.at.Compare(b.at) })
	return out
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// FriendlyName is what the page calls a client.
func FriendlyName(id string) string {
	low := strings.ToLower(id)
	switch {
	case strings.HasPrefix(id, "local-agent-mode-"):
		return "Claude Cowork"
	case id == "claude-ai":
		return "Claude Desktop"
	case id == "openai-mcp", strings.Contains(low, "chatgpt"):
		return "ChatGPT"
	case strings.HasPrefix(low, "codex"):
		return "Codex"
	}
	return id
}

// shortID is the 8-character prefix Claude Code prints.
func shortID(id string) string {
	r := []rune(id)
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}

// BuildStats answers GET /api/stats for rng at now. Bucket boundaries are local to loc.
func BuildStats(raw []mcpserver.Entry, rng string, now time.Time, loc *time.Location, lookup Lookup) (Stats, error) {
	span, ok := spans[rng]
	if !ok {
		return Stats{}, ErrRange
	}
	all := parse(raw, now)
	var first time.Time
	if len(all) > 0 {
		first = all[0].at
	}
	// "all" starts at the first call, but at most maxAllSpan back; with no calls, now.
	start := now.Add(-span)
	if span == 0 {
		start = now
		if !first.IsZero() {
			start = first
			if floor := now.Add(-maxAllSpan); start.Before(floor) {
				start = floor
			}
		}
	}
	var in, prev []entry
	for _, e := range all {
		switch {
		case e.at.After(now):
		case !e.at.Before(start):
			in = append(in, e)
		case span > 0 && !e.at.Before(start.Add(-span)):
			prev = append(prev, e)
		}
	}

	st := Stats{
		Range:       rng,
		GeneratedAt: stamp(now),
		Empty:       len(raw) == 0,
		KPIs:        KPIs{Window: window(in)},
		Clients:     clients(all, in),
		MostRead:    mostRead(in, lookup),
		TopQueries:  []Query{},
		Tools:       []Tool{},
		Recent:      recent(in),
	}
	if span > 0 {
		p := window(prev)
		st.KPIs.Prev = &p
	}
	st.Series = series(in, span == 24*time.Hour, start, now, first, loc)

	// Tools and queries count the same way as `unpause mcp stats`.
	ins := make([]mcpserver.Entry, len(in))
	for i, e := range in {
		ins[i] = *e.Entry
	}
	sum := mcpserver.Summarize(ins, time.Time{})
	for _, t := range sum.Tools {
		st.Tools = append(st.Tools, Tool{Name: t.Name, Count: t.Count})
	}
	for _, q := range sum.TopQueries {
		st.TopQueries = append(st.TopQueries, Query{Query: q.Name, Count: q.Count})
	}
	return st, nil
}

func window(es []entry) Window {
	w := Window{Calls: len(es)}
	clients, read := map[string]bool{}, map[string]bool{}
	var durs []int64
	for _, e := range es {
		clients[e.client] = true
		if e.Error != "" {
			w.Errors++
			continue
		}
		durs = append(durs, e.DurationMS)
		if e.Tool == "get_session" {
			for _, id := range e.SessionIDs {
				read[id] = true
			}
		}
	}
	w.Clients, w.SessionsRead = len(clients), len(read)
	if len(durs) > 0 {
		slices.Sort(durs)
		p := durs[(len(durs)-1)/2]
		w.P50MS = &p
	}
	return w
}

// clients lists the clients with calls in the range, most calls first, each with its
// Slot from the whole log (all is oldest first).
func clients(all, in []entry) []Client {
	slot := map[string]int{}
	for _, e := range all {
		if _, ok := slot[e.client]; !ok {
			s := len(slot)
			if s >= maxSlots {
				s = OtherSlot
			}
			slot[e.client] = s
		}
	}
	byID := map[string]*Client{}
	read := map[string]map[string]bool{}
	out := []Client{}
	var order []string
	for _, e := range in {
		c, ok := byID[e.client]
		if !ok {
			c = &Client{ID: e.client, Name: FriendlyName(e.client), Slot: slot[e.client]}
			byID[e.client] = c
			order = append(order, e.client)
		}
		c.Calls++
		c.LastSeen = stamp(e.at)
		if e.Tool == "get_session" && e.Error == "" {
			for _, id := range e.SessionIDs {
				if !read[e.client][id] {
					if read[e.client] == nil {
						read[e.client] = map[string]bool{}
					}
					read[e.client][id] = true
					c.SessionsRead++
				}
			}
		}
	}
	for _, id := range order {
		out = append(out, *byID[id])
	}
	slices.SortStableFunc(out, func(a, b Client) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// series buckets the calls in range. 24h is hourly from the hour holding the window start
// through the current hour (24 or 25 points, so the series sums to kpis.calls); other
// ranges are daily from the local day holding start through today. A bucket that ends
// at or before the first call in the whole log is not covered.
func series(in []entry, hourly bool, start, now, first time.Time, loc *time.Location) Series {
	s := Series{Bucket: "day", Points: []Point{}}
	var starts []time.Time
	l := start.In(loc)
	if hourly {
		s.Bucket = "hour"
		for b := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, loc); !b.After(now); b = b.Add(time.Hour) {
			starts = append(starts, b)
		}
	} else {
		for i := 0; ; i++ {
			b := time.Date(l.Year(), l.Month(), l.Day()+i, 0, 0, 0, 0, loc)
			if b.After(now) {
				break
			}
			starts = append(starts, b)
		}
	}
	for i, b := range starts {
		var end time.Time
		switch {
		case i+1 < len(starts):
			end = starts[i+1]
		case hourly:
			end = b.Add(time.Hour)
		default:
			bl := b.In(loc)
			end = time.Date(bl.Year(), bl.Month(), bl.Day()+1, 0, 0, 0, 0, loc)
		}
		s.Points = append(s.Points, Point{T: stamp(b), Covered: !first.IsZero() && end.After(first), ByClient: map[string]int{}})
	}
	for _, e := range in {
		i, found := slices.BinarySearchFunc(starts, e.at, func(b, t time.Time) int { return b.Compare(t) })
		if !found {
			i--
		}
		if i >= 0 && i < len(s.Points) {
			s.Points[i].ByClient[e.client]++
		}
	}
	return s
}

// mostRead is the top Sessions by successful get_session calls, titled from the index.
// lookup is only called for the Sessions shown.
func mostRead(in []entry, lookup Lookup) []Read {
	byID := map[string]*Read{}
	for _, e := range in {
		if e.Tool != "get_session" || e.Error != "" {
			continue
		}
		for _, id := range e.SessionIDs {
			r, ok := byID[id]
			if !ok {
				r = &Read{ID: id}
				byID[id] = r
			}
			r.Reads++
			r.LastRead = stamp(e.at)
		}
	}
	out := []Read{}
	for _, r := range byID {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Read) int {
		return cmp.Or(cmp.Compare(b.Reads, a.Reads), cmp.Compare(b.LastRead, a.LastRead), cmp.Compare(a.ID, b.ID))
	})
	if len(out) > topN {
		out = out[:topN]
	}
	for i := range out {
		out[i].Title = shortID(out[i].ID)
		if info, ok := lookup(out[i].ID); ok {
			out[i].Title, out[i].Account, out[i].Project = cmp.Or(info.Title, out[i].Title), info.Account, info.Project
		}
	}
	return out
}

// recent is the newest calls in range, newest first, input passed through as logged.
func recent(in []entry) []Call {
	out := []Call{}
	for i := len(in) - 1; i >= 0 && len(out) < recentN; i-- {
		e := in[i]
		input := e.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		ids := e.SessionIDs
		if ids == nil {
			ids = []string{}
		}
		out = append(out, Call{
			At: stamp(e.at), Client: e.client, ClientName: FriendlyName(e.client), Tool: e.Tool, Input: input,
			SessionIDs: ids, Count: e.Count, Total: e.Total, DurationMS: e.DurationMS, Error: e.Error,
		})
	}
	return out
}

// SessionCall is one logged call that returned a Session.
type SessionCall struct {
	At         string `json:"at"`
	ClientName string `json:"client_name"`
	Tool       string `json:"tool"`
	Count      int    `json:"count"`
}

// Session is the body of GET /api/session.
type Session struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Account string        `json:"account"`
	Project string        `json:"project"`
	Calls   []SessionCall `json:"calls"`
}

// BuildSession answers GET /api/session: every logged call that returned id (an exact
// id), newest first, with the same sane times as BuildStats at now. ok is false when no call did, so the API never describes a Session
// the log doesn't name.
func BuildSession(raw []mcpserver.Entry, id string, now time.Time, lookup Lookup) (Session, bool) {
	all := parse(raw, now)
	s := Session{ID: id, Title: shortID(id), Calls: []SessionCall{}}
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if e.Error != "" || !slices.Contains(e.SessionIDs, id) {
			continue
		}
		s.Calls = append(s.Calls, SessionCall{At: stamp(e.at), ClientName: FriendlyName(e.client), Tool: e.Tool, Count: e.Count})
	}
	if len(s.Calls) == 0 {
		return Session{}, false
	}
	if info, ok := lookup(id); ok {
		s.Title, s.Account, s.Project = cmp.Or(info.Title, s.Title), info.Account, info.Project
	}
	return s, true
}
