package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRootsAndLabels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	for _, d := range []string{".claude/projects", ".claude-work/projects", ".claude-nope", ".claudia/projects"} {
		os.MkdirAll(filepath.Join(home, d), 0o755)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	roots := c.ResolveRoots()
	if len(roots) != 2 {
		t.Fatalf("roots = %+v", roots)
	}
	if roots[0].Label != "personal" || roots[1].Label != "work" {
		t.Errorf("labels = %q, %q", roots[0].Label, roots[1].Label)
	}
}

func TestConfigOverridesLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	os.MkdirAll(filepath.Join(home, ".claude-work", "projects"), 0o755)
	os.MkdirAll(filepath.Join(home, "elsewhere", "projects"), 0o755)
	os.MkdirAll(filepath.Join(xdg, "unpause"), 0o755)
	os.WriteFile(filepath.Join(xdg, "unpause", "config.toml"), []byte(`
opener = "tmux"
[[roots]]
label = "client"
path = "~/.claude-work"
[[roots]]
path = "~/elsewhere"
`), 0o644)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Opener != "tmux" {
		t.Errorf("opener = %q", c.Opener)
	}
	roots := c.ResolveRoots()
	got := map[string]string{}
	for _, r := range roots {
		got[filepath.Base(r.Path)] = r.Label
	}
	if got[".claude-work"] != "client" || got["elsewhere"] != "elsewhere" {
		t.Errorf("roots = %+v", roots)
	}
}

func TestEnvForDefaultRootIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if EnvFor(filepath.Join(home, ".claude")) != "" {
		t.Error("default root must launch with CLAUDE_CONFIG_DIR unset")
	}
	if EnvFor(filepath.Join(home, ".claude-work")) != filepath.Join(home, ".claude-work") {
		t.Error("non-default root must be passed through")
	}
}
