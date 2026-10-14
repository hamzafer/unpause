package session

import (
	"os"
	"path/filepath"
	"strings"
)

// worktreesDir is where Claude Code puts the worktrees it creates inside a repo.
var worktreesDir = string(filepath.Separator) + filepath.Join(".claude", "worktrees") + string(filepath.Separator)

// splitWorktree splits <repo>/.claude/worktrees/<name>[/<sub...>] into repo and sub.
// ok is false when cwd is not inside a Claude Code worktree.
func splitWorktree(cwd string) (repo, sub string, ok bool) {
	cwd = filepath.Clean(cwd)
	i := strings.LastIndex(cwd, worktreesDir)
	if i <= 0 {
		return "", "", false
	}
	repo = cwd[:i]
	rest := cwd[i+len(worktreesDir):]
	name, sub, _ := strings.Cut(rest, string(filepath.Separator))
	if name == "" {
		return "", "", false
	}
	return repo, sub, true
}

// RetiredWorktreeRoot returns the repo a missing cwd belonged to when that cwd was a
// Claude Code worktree (<repo>/.claude/worktrees/<name>, or a folder inside one) whose
// repo still exists. It returns "" for every other path. Callers check that cwd itself
// is missing; this only looks at the path and the repo.
func RetiredWorktreeRoot(cwd string) string {
	repo, _, ok := splitWorktree(cwd)
	if !ok || !isDir(repo) {
		return ""
	}
	return repo
}

// OpenDir is the folder a resumed Session should start in: its cwd when that exists,
// the repo (or the same subfolder of it) for a Retired worktree, else home for an Orphan.
// Claude Code resumes the transcript regardless of cwd.
func (s *Session) OpenDir(home string) string {
	if s.WorktreeRoot != "" {
		if _, sub, ok := splitWorktree(s.CWD); ok && sub != "" {
			if d := filepath.Join(s.WorktreeRoot, sub); isDir(d) {
				return d
			}
		}
		return s.WorktreeRoot
	}
	if s.CWDMissing || s.CWD == "" {
		return home
	}
	return s.CWD
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
