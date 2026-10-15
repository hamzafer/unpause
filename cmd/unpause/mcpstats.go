package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/mcpserver"
	"github.com/hamzafer/unpause/internal/session"
)

// mcpStatsCmd is `unpause mcp stats`: a summary of the Access log.
func mcpStatsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stats",
		Short: "Summarize what MCP clients read",
		Long: "Reads the access log the MCP server keeps next to the index cache (access.jsonl, plus access.jsonl.1 after a rotation) " +
			"and shows calls per tool and per client, the sessions read most through get_session, errors and the top search queries. " +
			"The log never holds message text or titles; titles shown here are looked up on this machine.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sinceFlag, _ := cmd.Flags().GetString("since")
			asJSON, _ := cmd.Flags().GetBool("json")
			window, err := parseSince(sinceFlag)
			if err != nil {
				return err
			}
			entries, err := mcpserver.ReadLog(mcpserver.LogPath(config.CachePath()))
			if err != nil {
				return err
			}
			var since time.Time
			if window > 0 {
				since = time.Now().Add(-window)
			}
			st := mcpserver.Summarize(entries, since)
			lookup := func(string) (string, string, bool) { return "", "", false }
			if len(st.MostRead) > 0 {
				// The titles come from this machine's index, never from the log.
				if _, sessions, err := loadAll(true); err == nil {
					lookup = func(id string) (string, string, bool) {
						for _, s := range sessions {
							if s.ID == id {
								return s.Account, s.Title(), true
							}
						}
						return "", "", false
					}
				}
			}
			view := statsView(st, lookup)
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(view)
			}
			switch {
			case len(entries) == 0:
				fmt.Println("no MCP calls logged yet")
			case st.Calls == 0:
				fmt.Printf("no MCP calls in the last %s\n", sinceFlag)
			default:
				writeStats(os.Stdout, view, sinceFlag)
			}
			return nil
		},
	}
	c.Flags().String("since", "7d", "only calls this recent: a Go duration (36h) or days (7d); 0 = all")
	c.Flags().Bool("json", false, "output JSON")
	return c
}

// parseSince reads a Go duration, with "d" for days allowed in front ("7d", "1d12h").
func parseSince(s string) (time.Duration, error) {
	in := s
	bad := fmt.Errorf("--since %q: want a duration like 7d, 36h or 1d12h", s)
	var days time.Duration
	if i := strings.Index(s, "d"); i >= 0 {
		n, err := strconv.Atoi(s[:i])
		if err != nil || n < 0 {
			return 0, bad
		}
		if n > maxSinceDays {
			return 0, fmt.Errorf("--since %q: too long, at most %dd", s, maxSinceDays)
		}
		days = time.Duration(n) * 24 * time.Hour
		s = s[i+1:]
		if s == "" {
			return days, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, bad
	}
	if d > math.MaxInt64-days {
		return 0, fmt.Errorf("--since %q: too long, at most %dd", in, maxSinceDays)
	}
	return days + d, nil
}

// maxSinceDays is the most whole days a time.Duration holds (about 292 years).
const maxSinceDays = int(math.MaxInt64 / int64(24*time.Hour))

// safe replaces control characters (ESC, BEL, C1 and the like) in text read from the log
// or the index, so printing it can't drive the terminal. Whitespace becomes a space.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r) && unicode.IsControl(r):
			return ' '
		case unicode.IsControl(r):
			return '?'
		}
		return r
	}, s)
}

// readCount is one Session in "most read", with its account and title when it still exists.
type readCount struct {
	ID      string `json:"id"`
	Count   int    `json:"count"`
	Account string `json:"account,omitempty"`
	Title   string `json:"title,omitempty"`
}

type statsOut struct {
	Calls      int               `json:"calls"`
	Errors     int               `json:"errors"`
	First      time.Time         `json:"first,omitzero"`
	Last       time.Time         `json:"last,omitzero"`
	Tools      []mcpserver.Tally `json:"tools"`
	Clients    []mcpserver.Tally `json:"clients"`
	MostRead   []readCount       `json:"most_read"`
	TopQueries []mcpserver.Tally `json:"top_queries"`
}

func statsView(st mcpserver.Stats, lookup func(id string) (account, title string, ok bool)) statsOut {
	out := statsOut{
		Calls: st.Calls, Errors: st.Errors, First: st.First, Last: st.Last,
		Tools: st.Tools, Clients: st.Clients, TopQueries: st.TopQueries, MostRead: []readCount{},
	}
	for _, r := range st.MostRead {
		rc := readCount{ID: r.Name, Count: r.Count}
		if acct, title, ok := lookup(r.Name); ok {
			rc.Account, rc.Title = acct, title
		}
		out.MostRead = append(out.MostRead, rc)
	}
	return out
}

func writeStats(w io.Writer, st statsOut, window string) {
	const stamp = "2006-01-02 15:04"
	span := "all time"
	if d, _ := parseSince(window); d > 0 {
		span = "last " + window
	}
	fmt.Fprintf(w, "calls    %d, %s to %s (%s)\n", st.Calls, st.First.Local().Format(stamp), st.Last.Local().Format(stamp), span)
	fmt.Fprintf(w, "errors   %d\n", st.Errors)
	section := func(title string, ts []mcpserver.Tally) {
		if len(ts) == 0 {
			return
		}
		fmt.Fprintf(w, "\n%s\n", title)
		// The name column fits the longest name in this section: at least 18, at most 40.
		names := make([]string, len(ts))
		width := 18
		for i, t := range ts {
			names[i] = session.Clip(safe(t.Name), 40)
			width = max(width, utf8.RuneCountInString(names[i]))
		}
		for i, t := range ts {
			fmt.Fprintf(w, "  %-*s%4d\n", width, names[i], t.Count)
		}
	}
	section("tools", st.Tools)
	section("clients", st.Clients)
	if len(st.MostRead) > 0 {
		fmt.Fprintf(w, "\nmost read (get_session)\n")
		for _, r := range st.MostRead {
			id := []rune(safe(r.ID))
			if len(id) > 8 {
				id = id[:8]
			}
			if r.Title == "" && r.Account == "" {
				fmt.Fprintf(w, "  %4d  %s\n", r.Count, string(id))
				continue
			}
			fmt.Fprintf(w, "  %4d  %-8s %-9s %s\n", r.Count, string(id), safe(r.Account), session.Clip(safe(r.Title), 60))
		}
	}
	if len(st.TopQueries) > 0 {
		fmt.Fprintf(w, "\ntop queries\n")
		for _, q := range st.TopQueries {
			fmt.Fprintf(w, "  %4d  %s\n", q.Count, session.Clip(safe(q.Name), 60))
		}
	}
}
