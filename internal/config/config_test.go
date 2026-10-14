package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestCoworkRootsComeLastAndAreMarked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	base := filepath.Join("Library", "Application Support", "Claude", "local-agent-mode-sessions")
	for _, d := range []string{
		".claude/projects",
		".claude-work/projects",
		filepath.Join(base, "a", "b", "local_1", ".claude", "projects"),
		filepath.Join(base, "a", "c", "local_2", ".claude", "projects"),
		filepath.Join(base, "a", "c", "local_3", ".claude"),              // no projects/
		filepath.Join(base, "skills-plugin", "x", "local_4", "projects"), // not a .claude root
		filepath.Join(base, "a", "b", "other_5", ".claude", "projects"),  // not local_*
	} {
		os.MkdirAll(filepath.Join(home, d), 0o755)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	roots := c.ResolveRoots()
	if len(roots) != 4 {
		t.Fatalf("roots = %+v", roots)
	}
	if roots[0].Label != "personal" || roots[1].Label != "work" || roots[0].Cowork || roots[1].Cowork {
		t.Errorf("first roots = %+v", roots[:2])
	}
	for _, r := range roots[2:] {
		if r.Label != "cowork" || !r.Cowork || filepath.Base(r.Path) != ".claude" {
			t.Errorf("cowork root = %+v", r)
		}
	}

	c.NoAutoDetect = true
	if got := c.ResolveRoots(); len(got) != 0 {
		t.Errorf("no_auto_detect must skip cowork roots too, got %+v", got)
	}
}

func TestMCPExcludeExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	os.MkdirAll(filepath.Join(xdg, "unpause"), 0o755)
	os.WriteFile(filepath.Join(xdg, "unpause", "config.toml"), []byte(`
[mcp]
exclude = ["~/developer/work/*", "*/secret-repo"]
`), 0o644)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(home, "developer/work/*"), "*/secret-repo"}
	if len(c.MCP.Exclude) != 2 || c.MCP.Exclude[0] != want[0] || c.MCP.Exclude[1] != want[1] {
		t.Errorf("exclude = %q, want %q", c.MCP.Exclude, want)
	}
}

func writeConfig(t *testing.T, body string) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	os.MkdirAll(filepath.Join(xdg, "unpause"), 0o755)
	os.WriteFile(filepath.Join(xdg, "unpause", "config.toml"), []byte(body), 0o644)
	return home
}

func TestMCPExcludeIsNormalised(t *testing.T) {
	home := writeConfig(t, `
[mcp]
exclude = ["~/developer/work/", "secret-repo/", "/x//y/../z/*"]
`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(home, "developer/work"), "secret-repo", "/x/z/*"}
	if strings.Join(c.MCP.Exclude, "|") != strings.Join(want, "|") {
		t.Errorf("exclude = %q, want %q", c.MCP.Exclude, want)
	}
}

func TestMCPExcludeBadPatternFailsLoad(t *testing.T) {
	writeConfig(t, `
[mcp]
exclude = ["~/ok/*", "/x/[work"]
`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "/x/[work") {
		t.Errorf("a bad exclude pattern must fail Load and name the pattern, got %v", err)
	}
}
