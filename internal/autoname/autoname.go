// Package autoname gives untitled sessions a short name from a small model.
//
// A session qualifies only when nobody named it and Claude Code did not title it
// either, so a name the user chose (or Claude's own auto title) is never replaced.
// The name is written with the same /rename record unpause already uses (ADR 0005).
package autoname

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

// GuardEnv is set in the naming child so a SessionEnd hook that somehow still runs
// inside it does nothing, instead of naming the naming call.
const GuardEnv = "UNPAUSE_AUTONAME"

// maxNameLen caps a generated name so a chatty reply can't flood the list.
const maxNameLen = 60

// callTimeout bounds one model call; a normal one takes under ten seconds.
const callTimeout = 90 * time.Second

const systemPrompt = "You name coding sessions for a session picker. Reply with only the name: " +
	"3 to 6 plain words saying what the session was about, no quotes, no trailing punctuation."

// Needs reports whether s should get a generated name.
func Needs(s *session.Session) bool {
	return !s.Named() && s.AITitle == "" && !s.Empty() && !s.Sidechain
}

// Prompt is what the model sees: the repo, the first prompt and the last few messages.
func Prompt(s *session.Session) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repo: %s\n", s.Repo())
	if s.Branch != "" {
		fmt.Fprintf(&b, "Branch: %s\n", s.Branch)
	}
	fmt.Fprintf(&b, "\nFirst prompt:\n%s\n", s.FirstPrompt)
	if len(s.Preview) > 0 {
		b.WriteString("\nLast messages:\n")
		for _, m := range s.Preview {
			fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Text)
		}
	}
	return b.String()
}

// Clean turns a model reply into a name: first line, no label, quotes or markdown, capped length.
// It returns "" when nothing usable is left.
func Clean(raw string) string {
	line := ""
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	for _, label := range []string{"name:", "title:"} {
		if strings.HasPrefix(strings.ToLower(line), label) {
			line = line[len(label):]
		}
	}
	const junk = " \t\"'`*_"
	line = strings.Trim(line, junk)
	line = strings.TrimRight(line, ".!")
	line = strings.Trim(line, junk)
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > maxNameLen {
		cut := line[:maxNameLen]
		if i := strings.LastIndexByte(cut, ' '); i > 0 {
			cut = cut[:i]
		}
		line = cut
	}
	return line
}

// Namer asks a model for a name. The reply may be messy; Run cleans it.
type Namer func(ctx context.Context, s *session.Session) (string, error)

// Result is the outcome for one session. Name is set even on a dry run.
type Result struct {
	Session *session.Session
	Name    string
	Err     error
}

// Run names each session in turn and, unless dryRun, writes the name with write.
// report is called once per session, in order.
func Run(ctx context.Context, sessions []*session.Session, namer Namer, write func(*session.Session, string) error, dryRun bool, report func(Result)) {
	for _, s := range sessions {
		r := Result{Session: s}
		raw, err := namer(ctx, s)
		switch {
		case err != nil:
			r.Err = err
		case Clean(raw) == "":
			r.Err = errors.New("model returned an empty name")
		default:
			r.Name = Clean(raw)
			if !dryRun {
				r.Err = write(s, r.Name)
			}
		}
		report(r)
	}
}

// Haiku names sessions with `claude -p --model haiku`, under the session's own account so it
// uses that account's login. The call keeps no transcript, loads no hooks, tools or MCP
// servers, and runs from the temp dir so no project CLAUDE.md is pulled in.
//
// Once an account fails to authenticate, its remaining sessions fail at once with the same
// error instead of each waiting on another doomed call.
func Haiku(claudeBin string, configDir func(*session.Session) string) Namer {
	loggedOut := map[string]error{}
	return func(ctx context.Context, s *session.Session) (string, error) {
		dir := configDir(s)
		if err := loggedOut[dir]; err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(ctx, callTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, claudeBin, "-p",
			"--model", "haiku",
			"--no-session-persistence",
			"--settings", `{"disableAllHooks":true}`,
			"--tools", "",
			"--strict-mcp-config",
			"--system-prompt", systemPrompt,
		)
		cmd.Dir = os.TempDir()
		cmd.Env = childEnv(os.Environ(), dir)
		cmd.Stdin = strings.NewReader(Prompt(s))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = strings.TrimSpace(string(out))
			}
			err = fmt.Errorf("claude -p: %w: %s", err, msg)
			if strings.Contains(strings.ToLower(msg), "authenticate") {
				loggedOut[dir] = err
			}
			return "", err
		}
		return string(out), nil
	}
}

// childEnv sets CLAUDE_CONFIG_DIR for the session's account (unset for the default one,
// so an inherited value can't leak in) and adds the recursion guard.
func childEnv(env []string, configDir string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") && !strings.HasPrefix(kv, GuardEnv+"=") {
			out = append(out, kv)
		}
	}
	if configDir != "" {
		out = append(out, "CLAUDE_CONFIG_DIR="+configDir)
	}
	return append(out, GuardEnv+"=1")
}

// HookInput is the part of Claude Code's SessionEnd hook payload we use.
type HookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Reason         string `json:"reason"`
}

// ReadHookInput parses the JSON a SessionEnd hook receives on stdin.
func ReadHookInput(r io.Reader) (HookInput, error) {
	var h HookInput
	if err := json.NewDecoder(r).Decode(&h); err != nil {
		return h, fmt.Errorf("reading hook input: %w", err)
	}
	if h.SessionID == "" {
		return h, errors.New("hook input has no session_id")
	}
	return h, nil
}

// LogPath is where the detached hook job writes what it did.
func LogPath(cacheFile string) string {
	return filepath.Join(filepath.Dir(cacheFile), "autoname.log")
}

// Spawn starts bin with args in the background, detached from the caller, with its output
// appended to logPath. It returns as soon as the child has started: SessionEnd hooks get
// about 1.5 seconds before Claude Code kills them, and a model call takes longer.
func Spawn(bin string, args []string, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
