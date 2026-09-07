// Package opener launches a resumed session in the right place: a new tab of the current
// terminal, a tmux window, or in place of the current process.
package opener

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Launch is everything needed to resume one session.
type Launch struct {
	Claude    string // binary name or path
	SessionID string
	CWD       string
	ConfigDir string // CLAUDE_CONFIG_DIR value
	Title     string // window/tab title
	Fork      bool   // pass --fork-session
	ExtraArgs []string
}

// Args builds the claude argv (without argv[0]).
func (l Launch) Args() []string {
	a := []string{"--resume", l.SessionID}
	if l.Fork {
		a = append(a, "--fork-session")
	}
	return append(a, l.ExtraArgs...)
}

// ShellCommand renders the launch as a single shell line, used by tab openers and `--print`.
func (l Launch) ShellCommand() string {
	parts := []string{"cd", shellQuote(l.CWD), "&&"}
	if l.ConfigDir != "" {
		parts = append(parts, "CLAUDE_CONFIG_DIR="+shellQuote(l.ConfigDir))
	}
	parts = append(parts, "exec", shellQuote(l.Claude))
	for _, a := range l.Args() {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// Opener is one way of starting a session.
type Opener interface {
	Name() string
	Open(l Launch) error
}

// Kind selects an opener: "auto", "tab", "tmux" or "inplace".
func Pick(kind string) (Opener, error) {
	switch kind {
	case "", "auto":
		return Detect(), nil
	case "tab":
		if o := terminalTab(); o != nil {
			return o, nil
		}
		return nil, fmt.Errorf("no tab opener for this terminal (TERM_PROGRAM=%q); use tmux or inplace", os.Getenv("TERM_PROGRAM"))
	case "tmux":
		return Tmux{}, nil
	case "inplace":
		return InPlace{}, nil
	}
	return nil, fmt.Errorf("unknown opener %q", kind)
}

// Detect picks the best opener for the terminal we're running in.
func Detect() Opener {
	if os.Getenv("TMUX") != "" {
		return Tmux{}
	}
	if o := terminalTab(); o != nil {
		return o
	}
	return InPlace{}
}

func terminalTab() Opener {
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty":
		if _, err := exec.LookPath("osascript"); err == nil {
			return Ghostty{}
		}
	}
	return nil
}

// InPlace replaces the current process with claude.
type InPlace struct{}

func (InPlace) Name() string { return "inplace" }

func (InPlace) Open(l Launch) error {
	bin, err := exec.LookPath(l.Claude)
	if err != nil {
		return fmt.Errorf("claude not found: %w", err)
	}
	if err := os.Chdir(l.CWD); err != nil {
		return err
	}
	env := os.Environ()
	if l.ConfigDir != "" {
		env = setEnv(env, "CLAUDE_CONFIG_DIR", l.ConfigDir)
	}
	return syscall.Exec(bin, append([]string{bin}, l.Args()...), env)
}

// Tmux opens a new tmux window; if no server is running it starts one and attaches.
type Tmux struct{}

func (Tmux) Name() string { return "tmux" }

func (Tmux) Open(l Launch) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	args := []string{"new-window", "-n", tmuxName(l.Title), "-c", l.CWD}
	if l.ConfigDir != "" {
		args = append(args, "-e", "CLAUDE_CONFIG_DIR="+l.ConfigDir)
	}
	args = append(args, l.ShellCommand())
	if os.Getenv("TMUX") == "" {
		// Outside tmux: create (or reuse) a session called "unpause" and attach to it.
		if exec.Command("tmux", "has-session", "-t", "unpause").Run() != nil {
			start := append([]string{"new-session", "-d", "-s", "unpause", "-n", tmuxName(l.Title), "-c", l.CWD}, l.ShellCommand())
			if out, err := exec.Command("tmux", start...).CombinedOutput(); err != nil {
				return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
			}
		} else {
			args = append([]string{args[0], "-t", "unpause:"}, args[1:]...)
			if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
			}
		}
		bin, _ := exec.LookPath("tmux")
		return syscall.Exec(bin, []string{"tmux", "attach", "-t", "unpause"}, os.Environ())
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func tmuxName(t string) string {
	t = strings.Map(func(r rune) rune {
		if r == '.' || r == ':' {
			return '-'
		}
		return r
	}, t)
	if len(t) > 24 {
		t = t[:24]
	}
	if t == "" {
		t = "claude"
	}
	return t
}

func setEnv(env []string, key, val string) []string {
	out := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+val)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
