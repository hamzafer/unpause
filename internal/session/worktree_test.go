package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredWorktreeRoot(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "here"), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(repo, ".claude", "worktrees")
	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"worktree", filepath.Join(wt, "pow-796"), repo},
		{"worktree subfolder", filepath.Join(wt, "pow-796", "api", "v2"), repo},
		{"repo missing", filepath.Join(tmp, "gone", ".claude", "worktrees", "pow-796"), ""},
		{"not a worktree", filepath.Join(tmp, "elsewhere", "project"), ""},
		{"worktrees dir itself", filepath.Join(repo, ".claude", "worktrees"), ""},
		{"cwd present", filepath.Join(repo, "here"), ""},
	}
	for _, c := range cases {
		// Mirrors internal/index: only a missing cwd is checked.
		got := ""
		if _, err := os.Stat(c.cwd); err != nil {
			got = RetiredWorktreeRoot(c.cwd)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestOpenDir(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(repo, ".claude", "worktrees", "pow-796")
	home := "/home/me"
	cases := []struct {
		name string
		s    Session
		want string
	}{
		{"retired, subfolder exists", Session{CWD: filepath.Join(wt, "api"), CWDMissing: true, WorktreeRoot: repo}, filepath.Join(repo, "api")},
		{"retired, subfolder gone", Session{CWD: filepath.Join(wt, "web"), CWDMissing: true, WorktreeRoot: repo}, repo},
		{"retired, worktree root", Session{CWD: wt, CWDMissing: true, WorktreeRoot: repo}, repo},
		{"orphan", Session{CWD: "/x/gone", CWDMissing: true}, home},
		{"no cwd", Session{}, home},
		{"cwd present", Session{CWD: repo}, repo},
	}
	for _, c := range cases {
		if got := c.s.OpenDir(home); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if Tilde("/home/me/dev/repo", home) != "~/dev/repo" || Tilde("/home/meow", home) != "/home/meow" || Tilde(home, home) != "~" {
		t.Error("Tilde formatting")
	}
}
