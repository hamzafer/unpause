// unpause: one list of every Claude Code session on your machine. Pick one, it opens where it left off.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/index"
	"github.com/hamzafer/unpause/internal/opener"
	"github.com/hamzafer/unpause/internal/provider/claude"
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
			res, err := tui.Run(tui.Options{
				Sessions: filter(sessions, all),
				Opener:   op,
				Claude:   cfg.Claude,
				Rename: func(s *session.Session, name string) error {
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
				if s.CWDMissing {
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
			openWith, _ := cmd.Flags().GetString("open")
			if openWith == "" {
				openWith = cfg.Opener
			}
			fork, _ := cmd.Flags().GetBool("fork")
			l := opener.Launch{Claude: cfg.Claude, SessionID: s.ID, CWD: s.CWD, ConfigDir: config.EnvFor(s.Root), Title: s.Title(), Fork: fork}
			if p, _ := cmd.Flags().GetBool("print"); p {
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
	open.Flags().StringP("open", "o", "", "how to open: auto, tab, tmux, inplace")
	open.Flags().Bool("fork", false, "resume as a fork (new session id)")
	open.Flags().Bool("print", false, "print the shell command instead of running it")

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
			name := strings.Join(args[1:], " ")
			if err := claude.Rename(s.Path, s.ID, name); err != nil {
				return err
			}
			index.Invalidate(s.Path)
			fmt.Printf("renamed %s → %q\n", s.ShortID(), name)
			return nil
		},
	}

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
			for _, r := range cfg.ResolveRoots() {
				ts, _ := claude.ListTranscripts(r)
				live := claude.LiveSessions(r.Path)
				fmt.Printf("  %-10s %s  (%d transcripts, %d live)\n", r.Label, r.Path, len(ts), len(live))
			}
			return nil
		},
	}

	root.AddCommand(list, open, rename, doctor)
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
	roots := cfg.ResolveRoots()
	if len(roots) == 0 {
		return nil, nil, fmt.Errorf("no Claude Code config dirs found (looked for ~/.claude and ~/.claude-*); add roots in %s", config.Path())
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
	sessions, err := index.Build(roots, progress)
	return cfg, sessions, err
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
	q = strings.ToLower(strings.TrimSpace(q))
	for _, s := range sessions {
		if strings.HasPrefix(strings.ToLower(s.ID), q) {
			return s
		}
	}
	for _, s := range sessions {
		if strings.ToLower(s.CustomTitle) == q {
			return s
		}
	}
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.Title()), q) {
			return s
		}
	}
	return nil
}
