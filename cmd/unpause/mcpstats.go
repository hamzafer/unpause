package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/mcpserver"
	"github.com/hamzafer/unpause/internal/session"
	"github.com/hamzafer/unpause/internal/statsweb"
	"github.com/hamzafer/unpause/internal/webui"
)

// mcpStatsCmd is `unpause mcp stats`: a summary of the Access log.
func mcpStatsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stats",
		Short: "Summarize what MCP clients read",
		Long: "Reads the access log the MCP server keeps next to the index cache (access.jsonl, plus access.jsonl.1 after a rotation) " +
			"and shows calls per tool and per client, the sessions read most through get_session, errors and the top search queries. " +
			"The log never holds message text or titles; titles shown here are looked up on this machine.\n\n" +
			"With --web it serves the same log as a live page on 127.0.0.1 and opens it in the browser " +
			"(--port picks the port, --no-open only prints the URL). Ctrl+C stops it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			web, _ := cmd.Flags().GetBool("web")
			if !web {
				for _, f := range []string{"port", "no-open"} {
					if cmd.Flags().Changed(f) {
						return fmt.Errorf("--%s only works with --web", f)
					}
				}
			}
			if web {
				port, _ := cmd.Flags().GetInt("port")
				noOpen, _ := cmd.Flags().GetBool("no-open")
				return runStatsWeb(cmd.Context(), cmd.ErrOrStderr(), port, !noOpen)
			}
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
	c.Flags().Bool("web", false, "show the stats as a live page in the browser, served on 127.0.0.1")
	c.Flags().Int("port", 4777, "with --web: the port to serve on (a free one if it is taken)")
	c.Flags().Bool("no-open", false, "with --web: print the URL without opening the browser")
	c.MarkFlagsMutuallyExclusive("web", "json")
	c.MarkFlagsMutuallyExclusive("web", "since")
	return c
}

// runStatsWeb serves the stats page until Ctrl+C. The URL goes to stderr; stdout stays empty.
func runStatsWeb(ctx context.Context, stderr io.Writer, port int, open bool) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("--port %d: want 0 to 65535", port)
	}
	token, err := statsweb.NewToken()
	if err != nil {
		return err
	}
	ln, err := statsweb.Listen(port)
	if err != nil {
		return err
	}
	// The token is in the URL only: the page reads it from there, and API calls without
	// it are refused, so another local user can't read titles off the port.
	url := "http://" + ln.Addr().String() + "/?t=" + token
	fmt.Fprintf(stderr, "unpause mcp stats: %s (Ctrl+C to stop)\n", url)
	if open {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(stderr, "unpause: could not open the browser: %v\n", err)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first Ctrl+C, stop catching signals: a second one kills at once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	err = statsweb.Serve(ctx, ln, statsweb.Config{
		ReadLog: func() ([]mcpserver.Entry, error) { return mcpserver.ReadLog(mcpserver.LogPath(config.CachePath())) },
		Lookup:  indexLookup,
		Now:     time.Now,
		Loc:     time.Local,
		Static:  webui.FS(),
		Token:   token,
	})
	if errors.Is(err, statsweb.ErrSlowShutdown) {
		// Stopping is what was asked for; a slow browser tab is not a failure.
		fmt.Fprintf(stderr, "unpause: %v\n", err)
		return nil
	}
	return err
}

// indexLookup loads this machine's index for titles; the log never holds them.
func indexLookup() statsweb.Lookup {
	_, sessions, err := loadAll(true)
	if err != nil {
		return nil
	}
	byID := make(map[string]statsweb.Info, len(sessions))
	for _, s := range sessions {
		project := s.Repo()
		if project == "?" {
			project = ""
		}
		byID[s.ID] = statsweb.Info{Title: s.Title(), Account: s.Account, Project: project}
	}
	return func(id string) (statsweb.Info, bool) {
		info, ok := byID[id]
		return info, ok
	}
}

// openBrowser opens url with the desktop's default browser.
func openBrowser(url string) error {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		return fmt.Errorf("don't know how to open a browser on %s", runtime.GOOS)
	}
	cmd := exec.Command(name, url)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
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
