// unpause: one list of every Claude Code session on your machine. Pick one, it opens where it left off.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/hamzafer/unpause/internal/autoname"
	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/index"
	"github.com/hamzafer/unpause/internal/mcpserver"
	"github.com/hamzafer/unpause/internal/opener"
	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/recap"
	"github.com/hamzafer/unpause/internal/session"
	"github.com/hamzafer/unpause/internal/tui"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "unpause",
		Short:   "Pick up any Claude Code session from anywhere",
		Long:    "unpause lists every Claude Code session across all your repos and accounts, and opens the one you pick right where it left off.",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, sessions, err := loadAll(false)
			if err != nil {
				return err
			}
			all, _ := cmd.Flags().GetBool("all")
			openWith, _ := cmd.Flags().GetString("open")
			if openWith == "" {
				openWith = cfg.Opener
			}
			op, err := opener.Pick(openWith)
			if err != nil {
				return err
			}
			shown := filter(sessions, all)
			var recapOf func(*session.Session) (recap.Recap, bool, bool)
			if cfg.Recap.Enabled {
				// A store that can't be read just means no boxes; the picker still opens.
				_ = recap.Migrate(recap.Path(config.CachePath()), sessions)
				recaps, _ := recap.Load(recap.Path(config.CachePath()))
				recapOf = func(s *session.Session) (recap.Recap, bool, bool) {
					r, ok := recaps.Get(s)
					return r, ok && recap.Outdated(r, s), ok
				}
			}
			res, err := tui.Run(tui.Options{
				Recap:    recapOf,
				Sessions: shown,
				Search:   claude.Search,
				Opener:   op,
				Claude:   cfg.Claude,
				Rename: func(s *session.Session, name string) error {
					if s.Cowork {
						return session.ErrCoworkRename
					}
					if err := claude.Rename(s.Path, s.ID, name); err != nil {
						return err
					}
					index.Invalidate(s.Path)
					return nil
				},
			})
			if err != nil {
				return err
			}
			if res.Launch != nil {
				return op.Open(*res.Launch)
			}
			return nil
		},
	}
	root.Flags().BoolP("all", "a", false, "include empty sessions and subagent transcripts")
	root.Flags().StringP("open", "o", "", "how to open: auto, tab, tmux, inplace (default from config)")

	list := &cobra.Command{
		Use:   "list",
		Short: "Print sessions as text or JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, sessions, err := loadAll(true)
			if err != nil {
				return err
			}
			all, _ := cmd.Flags().GetBool("all")
			asJSON, _ := cmd.Flags().GetBool("json")
			sessions = filter(sessions, all)
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sessions)
			}
			for _, s := range sessions {
				live := " "
				if s.Live != nil {
					live = "●"
				}
				flag := ""
				switch {
				case s.WorktreeRoot != "":
					flag = " (worktree retired)"
				case s.CWDMissing:
					flag = " (cwd missing)"
				}
				age := session.Age(s.LastActive)
				if s.WillOfferSummary(time.Now()) {
					age += "◷"
				}
				fmt.Printf("%s %-8s %-9s %-28s %-6s %s%s\n", live, s.ShortID(), s.Account, session.Clip(s.Repo(), 28), age, session.Clip(s.Title(), 60), flag)
			}
			return nil
		},
	}
	list.Flags().BoolP("all", "a", false, "include empty sessions and subagent transcripts")
	list.Flags().Bool("json", false, "output JSON")

	search := &cobra.Command{
		Use:   "search <query...>",
		Short: "Deep search: find sessions by what was said in them",
		Long: "Searches the talk of every session (your prompts, Claude's replies, and the name or generated title), " +
			"not tool calls, tool output or thinking. The arguments are joined with spaces; every word must appear somewhere " +
			"in a session, and \"double quotes\" keep a phrase together. Results are ranked by match count, newest first on ties.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, sessions, err := loadAll(true)
			if err != nil {
				return err
			}
			all, _ := cmd.Flags().GetBool("all")
			asJSON, _ := cmd.Flags().GetBool("json")
			limit, _ := cmd.Flags().GetInt("limit")
			results, err := claude.Search(cmd.Context(), filter(sessions, all), searchQuery(args), nil)
			if err != nil {
				return err
			}
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}
			if asJSON {
				if results == nil {
					results = []claude.Result{}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			for _, r := range results {
				s := r.Session
				live := " "
				if s.Live != nil {
					live = "●"
				}
				fmt.Printf("%s%4s  %-8s  %-9s %-24s %-4s %s\n", live, fmt.Sprintf("%d×", r.Hits), s.ShortID(), s.Account,
					session.Clip(s.Repo(), 24), session.Age(s.LastActive), session.Clip(s.Title(), 60))
				if len(r.Snippets) > 0 {
					fmt.Printf("       %s\n", r.Snippets[0])
				}
			}
			return nil
		},
	}
	search.Flags().BoolP("all", "a", false, "include empty sessions and subagent transcripts")
	search.Flags().Bool("json", false, "output JSON: [{session, hits, snippets}]")
	search.Flags().Int("limit", 20, "show at most this many results (0 = all)")

	open := &cobra.Command{
		Use:   "open <id-or-name>",
		Short: "Resume one session by id prefix or name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, sessions, err := loadAll(true)
			if err != nil {
				return err
			}
			s := find(sessions, args[0])
			if s == nil {
				return fmt.Errorf("no session matches %q", args[0])
			}
			if s.Cowork {
				return session.ErrCowork
			}
			openWith, _ := cmd.Flags().GetString("open")
			if openWith == "" {
				openWith = cfg.Opener
			}
			fork, _ := cmd.Flags().GetBool("fork")
			l := opener.Launch{Claude: cfg.Claude, SessionID: s.ID, CWD: s.OpenDir(homeDir()), ConfigDir: config.EnvFor(s.Root), Title: s.Title(), Fork: fork}
			if p, _ := cmd.Flags().GetBool("print"); p {
				if j, _ := cmd.Flags().GetBool("json"); j {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(struct {
						Claude    string   `json:"claude"`
						SessionID string   `json:"session_id"`
						CWD       string   `json:"cwd"`
						ConfigDir string   `json:"config_dir,omitempty"`
						Fork      bool     `json:"fork"`
						Args      []string `json:"args"`
						Shell     string   `json:"shell"`
					}{l.Claude, l.SessionID, l.CWD, l.ConfigDir, l.Fork, l.Args(), l.ShellCommand()})
				}
				fmt.Println(l.ShellCommand())
				return nil
			}
			op, err := opener.Pick(openWith)
			if err != nil {
				return err
			}
			return op.Open(l)
		},
	}
	open.Flags().StringP("open", "o", "", "how to open: auto, tab, tmux, inplace (default from config)")
	open.Flags().Bool("fork", false, "resume as a fork (new session id)")
	open.Flags().Bool("print", false, "print the shell command instead of running it")
	open.Flags().Bool("json", false, "with --print, emit structured JSON instead of a shell line")

	rename := &cobra.Command{
		Use:   "rename <id-or-name> <new name>",
		Short: "Name a session (same record Claude Code writes for /rename)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, sessions, err := loadAll(true)
			if err != nil {
				return err
			}
			s := find(sessions, args[0])
			if s == nil {
				return fmt.Errorf("no session matches %q", args[0])
			}
			if s.Cowork {
				return session.ErrCoworkRename
			}
			name := strings.Join(args[1:], " ")
			if err := claude.Rename(s.Path, s.ID, name); err != nil {
				return err
			}
			index.Invalidate(s.Path)
			fmt.Printf("renamed %s → %q\n", s.ShortID(), name)
			return nil
		},
	}

	autonameCmd := &cobra.Command{
		Use:   "autoname",
		Short: "Name untitled sessions with Claude Haiku",
		Long: "Names every session that has neither a name you gave it nor a title Claude generated, " +
			"using `claude -p --model haiku` under the session's own account. Live sessions are skipped. " +
			"Use --dry-run first to see the proposed names, then --apply to write exactly those.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			apply, _ := cmd.Flags().GetBool("apply")
			only, _ := cmd.Flags().GetString("session")
			delay, _ := cmd.Flags().GetDuration("delay")
			if dryRun && apply {
				return fmt.Errorf("--dry-run and --apply don't go together")
			}
			if only != "" && (dryRun || apply) {
				return fmt.Errorf("--session is for the SessionEnd hook and can't be combined with --dry-run or --apply")
			}
			time.Sleep(delay)
			cfg, sessions, err := loadAll(true)
			if err != nil {
				return err
			}
			errTitled := errors.New("got a name or title in the meantime, left alone")
			write := func(s *session.Session, name string) error {
				if s.Cowork {
					// Claude Desktop owns Cowork transcripts; a stale --apply must not write one.
					return session.ErrCoworkRename
				}
				if ok, err := autoname.StillNeeds(s.Path); err != nil {
					return err
				} else if !ok {
					return errTitled
				}
				if err := claude.Rename(s.Path, s.ID, name); err != nil {
					return err
				}
				index.Invalidate(s.Path)
				return nil
			}
			line := func(stamp string, s *session.Session, what string) {
				fmt.Printf("%s%s %-9s %-24s %s\n", stamp, s.ShortID(), s.Account, session.Clip(s.Repo(), 24), what)
			}
			proposalsPath := autoname.ProposalsPath(config.CachePath())
			if apply {
				ps, err := autoname.LoadProposals(proposalsPath)
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("no saved names; run `unpause autoname --dry-run` first")
				} else if err != nil {
					return err
				}
				byID := map[string]*session.Session{}
				for _, s := range sessions {
					byID[s.ID] = s
				}
				// Names that couldn't be written yet stay saved for the next --apply;
				// written ones and ones the session no longer needs are dropped.
				var keep []autoname.Proposal
				done, leftAlone := 0, 0
				for _, p := range ps {
					s := byID[p.SessionID]
					if s == nil {
						s = &session.Session{ID: p.SessionID, Path: p.Path}
					}
					if s.Live != nil {
						keep = append(keep, p)
						line("", s, "● running now, kept for the next --apply")
						continue
					}
					switch err := write(s, p.Name); {
					case errors.Is(err, errTitled), errors.Is(err, session.ErrCoworkRename):
						leftAlone++
						line("", s, "– "+err.Error())
					case err != nil:
						keep = append(keep, p)
						line("", s, "✗ "+err.Error()+" (kept for the next --apply)")
					default:
						done++
						line("", s, fmt.Sprintf("→ %q", p.Name))
					}
				}
				fmt.Printf("named %d, left alone %d, kept %d\n", done, leftAlone, len(keep))
				if len(keep) > 0 {
					return autoname.SaveProposals(proposalsPath, keep)
				}
				if err := os.Remove(proposalsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				return nil
			}
			if dryRun {
				// Whatever this dry run finds replaces the last one, even when it finds nothing.
				if err := os.Remove(proposalsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			var todo []*session.Session
			for _, s := range sessions {
				switch {
				case only != "":
					// Hook mode: the session just ended, so its live marker may still be settling.
					if s.ID == only && autoname.Needs(s) {
						todo = append(todo, s)
					}
				case autoname.Needs(s) && s.Live == nil:
					todo = append(todo, s)
				}
			}
			stamp := ""
			if only != "" {
				stamp = time.Now().Format("2006-01-02 15:04") + " "
				if len(todo) == 0 {
					fmt.Printf("%s%s: already named or titled, nothing to do\n", stamp, only)
					return nil
				}
			}
			if len(todo) == 0 {
				fmt.Println("every session already has a name or a Claude title")
				return nil
			}
			if dryRun {
				fmt.Printf("dry run: asking Haiku for %d names, nothing will be written\n", len(todo))
			}
			namer := autoname.Haiku(cfg.Claude, func(s *session.Session) string { return config.EnvFor(s.Root) })
			done, failed := 0, 0
			var proposals []autoname.Proposal
			autoname.Run(cmd.Context(), todo, namer, write, dryRun, func(r autoname.Result) {
				if r.Err != nil {
					failed++
					line(stamp, r.Session, "✗ "+r.Err.Error())
					return
				}
				done++
				proposals = append(proposals, autoname.Proposal{SessionID: r.Session.ID, Path: r.Session.Path, Name: r.Name})
				line(stamp, r.Session, fmt.Sprintf("→ %q", r.Name))
			})
			verb := "named"
			if dryRun {
				verb = "would name"
			}
			fmt.Printf("%s%s %d, failed %d\n", stamp, verb, done, failed)
			if dryRun && len(proposals) > 0 {
				if err := autoname.SaveProposals(proposalsPath, proposals); err != nil {
					return err
				}
				fmt.Println("saved; run `unpause autoname --apply` to write exactly these names")
			}
			if failed > 0 && done == 0 {
				return fmt.Errorf("every naming call failed")
			}
			return nil
		},
	}
	autonameCmd.Flags().Bool("dry-run", false, "show proposed names and save them for --apply, without writing")
	autonameCmd.Flags().Bool("apply", false, "write the names the last --dry-run saved, without asking Haiku again")
	autonameCmd.Flags().String("session", "", "name only this session id (used by the SessionEnd hook)")
	autonameCmd.Flags().Duration("delay", 0, "wait this long before starting")
	_ = autonameCmd.Flags().MarkHidden("delay")

	recapCmd := &cobra.Command{
		SilenceUsage: true, // a failed model call is not a usage mistake
		Use:          "recap",
		Short:        "Write a Recap (goal, done, next) for a session with Claude Haiku",
		Long: "Asks `claude -p --model haiku`, under the session's own account, for three short lines about a session: " +
			"goal, done and next. The picker shows them in a box in the preview. Only the session's talk is sent " +
			"(prompts and replies, not tool output), about 8,000 characters at most. Recaps are kept in recaps.json " +
			"next to the index cache, never in the transcript. Off unless [recap] enabled = true is in the config.\n\n" +
			"Without --session it catches up: sessions with no Recap or an outdated one, newest first, at most --limit " +
			"of them. Running, Cowork, empty and subagent sessions are skipped.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if !cfg.Recap.Enabled {
				return fmt.Errorf("recaps are off. Each one sends part of a session's talk to Claude Haiku; to turn them on, add\n\n"+
					"  [recap]\n  enabled = true\n\nto %s", config.Path())
			}
			only, _ := cmd.Flags().GetString("session")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			limit, _ := cmd.Flags().GetInt("limit")
			if only != "" && (dryRun || cmd.Flags().Changed("limit")) {
				return fmt.Errorf("--session can't be combined with --dry-run or --limit")
			}
			delay, _ := cmd.Flags().GetDuration("delay")
			time.Sleep(delay)
			sessions, err := loadSessions(cfg, true)
			if err != nil {
				return err
			}
			ask := autoname.Ask(cfg.Claude, func(s *session.Session) string { return config.EnvFor(s.Root) }, recap.SystemPrompt)
			if only == "" {
				return recapBackfill(cmd.Context(), sessions, ask, limit, dryRun)
			}
			transcript, _ := cmd.Flags().GetString("transcript")
			s := recapTarget(sessions, only, transcript)
			if s == nil {
				return fmt.Errorf("no session matches %q", only)
			}
			r, err := recap.Make(cmd.Context(), s, ask)
			if errors.Is(err, recap.ErrLive) && delay > 0 {
				// Only the SessionEnd hook passes --delay. A Session still running then (it
				// was cleared or resumed elsewhere) gets its Recap at its next end.
				fmt.Printf("%s skipped: %v\n", s.ShortID(), err)
				return nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", s.ShortID(), err)
			}
			if err := recap.Put(recap.Path(config.CachePath()), recap.Key(s), r); err != nil {
				return err
			}
			fmt.Printf("%s %-9s %s\n  goal  %s\n  done  %s\n  next  %s\n", s.ShortID(), s.Account, session.Clip(s.Title(), 60), r.Goal, r.Done, r.Next)
			return nil
		},
	}
	recapCmd.Flags().String("session", "", "write the Recap for this session (id prefix or name)")
	recapCmd.Flags().Int("limit", 20, "without --session: recap at most this many sessions, newest first (0 = all)")
	recapCmd.Flags().Bool("dry-run", false, "without --session: list the sessions that would get a Recap, and stop")
	recapCmd.Flags().Duration("delay", 0, "wait this long before starting")
	_ = recapCmd.Flags().MarkHidden("delay")
	recapCmd.Flags().String("transcript", "", "with --session: the transcript that ended (the SessionEnd hook passes it)")
	_ = recapCmd.Flags().MarkHidden("transcript")

	hook := &cobra.Command{
		Use:   "hook",
		Short: "Entry points for Claude Code hooks",
	}
	sessionEnd := &cobra.Command{
		Use:   "session-end",
		Short: "SessionEnd hook: name the session that just ended, in the background",
		Long: "Reads Claude Code's SessionEnd hook input on stdin and starts a detached " +
			"`unpause autoname --session <id>` (and `unpause recap --session <id>` when Recaps are on), then returns at once. " +
			"Output goes to autoname.log next to the index cache.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv(autoname.GuardEnv) != "" {
				return nil
			}
			in, err := autoname.ReadHookInput(os.Stdin)
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			// A config that can't be read still names the Session; only the Recap waits for a fix.
			cfg, err := config.Load()
			for _, job := range sessionEndJobs(in.SessionID, in.TranscriptPath, err == nil && cfg.Recap.Enabled) {
				if err := autoname.Spawn(self, job, autoname.LogPath(config.CachePath())); err != nil {
					return err
				}
			}
			return nil
		},
	}
	hook.AddCommand(sessionEnd)

	doctor := &cobra.Command{
		Use:   "doctor",
		Short: "Show detected roots, opener and claude binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Printf("config:  %s\n", config.Path())
			fmt.Printf("cache:   %s\n", config.CachePath())
			if p, err := exec.LookPath(cfg.Claude); err == nil {
				fmt.Printf("claude:  %s\n", p)
			} else {
				fmt.Printf("claude:  NOT FOUND (%s)\n", cfg.Claude)
			}
			fmt.Printf("opener:  %s (config: %s, TERM_PROGRAM=%s, TMUX=%v)\n", opener.Detect().Name(), cfg.Opener, os.Getenv("TERM_PROGRAM"), os.Getenv("TMUX") != "")
			fmt.Println("roots:")
			coworkRoots, coworkTranscripts := 0, 0
			for _, r := range cfg.ResolveRoots() {
				ts, _ := claude.ListTranscripts(r)
				if r.Cowork {
					coworkRoots++
					coworkTranscripts += len(ts)
					continue
				}
				live := claude.LiveSessions(r.Path)
				fmt.Printf("  %-10s %s  (%d transcripts, %d live)\n", r.Label, r.Path, len(ts), len(live))
			}
			if coworkRoots > 0 {
				fmt.Printf("  %-10s %d roots, %d transcripts\n", config.CoworkLabel, coworkRoots, coworkTranscripts)
			}
			fmt.Printf("recaps:  %s\n", recapStatus(cfg))
			return nil
		},
	}

	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve sessions read-only over MCP on stdio",
		Long: "Runs a read-only MCP server on stdin/stdout with three tools: list_sessions, search_sessions and get_session. " +
			"It never writes to a transcript. Sessions that worked in a folder matching an [mcp] exclude glob in the config are invisible to it, " +
			"and so are sessions with no folder at all while any exclude is set. A broken exclude pattern stops it from starting. " +
			"Every tool call is logged, without chat content, to access.jsonl next to the index cache; `unpause mcp stats` summarizes it. " +
			"Meant to be started by an MCP client, such as OpenAI's tunnel-client for ChatGPT.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check the config now: a bad exclude pattern must stop the server, not fail every call.
			if _, err := config.Load(); err != nil {
				return err
			}
			// Tool calls may run concurrently; one index build at a time keeps the cache file sane.
			var mu sync.Mutex
			accessLog := mcpserver.NewAccessLog(mcpserver.LogPath(config.CachePath()))
			return mcpserver.Run(cmd.Context(), version, func(context.Context) ([]*session.Session, []string, error) {
				mu.Lock()
				defer mu.Unlock()
				cfg, sessions, err := loadAll(true)
				if err != nil {
					return nil, nil, err
				}
				return filter(sessions, false), cfg.MCP.Exclude, nil
			}, accessLog.Record)
		},
	}
	mcpCmd.AddCommand(mcpStatsCmd())

	root.AddCommand(list, search, open, rename, autonameCmd, recapCmd, hook, doctor, mcpCmd)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "unpause:", err)
		os.Exit(1)
	}
}

func loadAll(quiet bool) (*config.Config, []*session.Session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	sessions, err := loadSessions(cfg, quiet)
	return cfg, sessions, err
}

// loadSessions indexes every Session under cfg's roots, for a caller that already has cfg.
func loadSessions(cfg *config.Config, quiet bool) ([]*session.Session, error) {
	roots := cfg.ResolveRoots()
	if len(roots) == 0 {
		return nil, fmt.Errorf("no Claude Code config dirs found (looked for ~/.claude and ~/.claude-*); add roots in %s", config.Path())
	}
	var progress index.Progress
	if !quiet {
		progress = func(n, total int) {
			if total >= 20 {
				fmt.Fprintf(os.Stderr, "\rindexing %d/%d", n, total)
				if n == total {
					fmt.Fprint(os.Stderr, "\r\033[K")
				}
			}
		}
	}
	return index.Build(roots, progress)
}

func filter(in []*session.Session, all bool) []*session.Session {
	if all {
		return in
	}
	out := in[:0:0]
	for _, s := range in {
		if s.Empty() || s.Sidechain {
			continue
		}
		out = append(out, s)
	}
	return out
}

func find(sessions []*session.Session, q string) *session.Session {
	return session.Find(sessions, q)
}

// homeDir is where an Orphan opens; Claude Code resumes the transcript regardless of cwd.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

// searchQuery joins search's args into one query. A single argument with spaces and no
// quotes of its own was quoted in the shell, so it is searched as one phrase.
func searchQuery(args []string) string {
	if len(args) == 1 && strings.ContainsAny(args[0], " \t") && !strings.Contains(args[0], `"`) {
		return `"` + args[0] + `"`
	}
	return strings.Join(args, " ")
}
