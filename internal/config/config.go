// Package config loads ~/.config/unpause/config.toml and discovers Claude Code config roots.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Root is one Claude Code config directory (what CLAUDE_CONFIG_DIR points at) with a human label.
type Root struct {
	Label string `toml:"label"`
	Path  string `toml:"path"`
}

// Config is the on-disk configuration. Every field is optional.
type Config struct {
	// Opener is "auto", "tab", "tmux" or "inplace".
	Opener string `toml:"opener"`
	// Claude is the binary to run; defaults to "claude" from PATH.
	Claude string `toml:"claude"`
	// Roots overrides or extends the auto-detected list.
	Roots []Root `toml:"roots"`
	// NoAutoDetect turns off the ~/.claude* scan; only Roots are used.
	NoAutoDetect bool `toml:"no_auto_detect"`
}

// Path returns the config file location.
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "unpause", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "unpause", "config.toml")
}

// CachePath returns where the session index cache lives.
func CachePath() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "unpause", "index.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "unpause", "index.json")
}

// Load reads the config file; a missing file yields defaults.
func Load() (*Config, error) {
	c := &Config{Opener: "auto", Claude: "claude"}
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := toml.Decode(string(b), c); err != nil {
		return nil, err
	}
	if c.Opener == "" {
		c.Opener = "auto"
	}
	if c.Claude == "" {
		c.Claude = "claude"
	}
	for i := range c.Roots {
		c.Roots[i].Path = expand(c.Roots[i].Path)
	}
	return c, nil
}

// ResolveRoots merges auto-detected ~/.claude* dirs, $CLAUDE_CONFIG_DIR and configured roots.
// Configured labels win over derived ones for the same path.
func (c *Config) ResolveRoots() []Root {
	byPath := map[string]Root{}
	order := []string{}
	add := func(r Root) {
		r.Path = filepath.Clean(r.Path)
		if _, seen := byPath[r.Path]; !seen {
			order = append(order, r.Path)
		}
		byPath[r.Path] = r
	}

	if !c.NoAutoDetect {
		for _, r := range detect() {
			add(r)
		}
	}
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		p := expand(env)
		if _, seen := byPath[filepath.Clean(p)]; !seen {
			add(Root{Label: labelFor(p), Path: p})
		}
	}
	for _, r := range c.Roots {
		if r.Label == "" {
			r.Label = labelFor(r.Path)
		}
		add(r)
	}

	out := make([]Root, 0, len(order))
	for _, p := range order {
		r := byPath[p]
		if hasProjects(r.Path) {
			out = append(out, r)
		}
	}
	return out
}

// detect finds ~/.claude and every ~/.claude-<suffix> directory that holds a projects/ folder.
func detect() []Root {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		return nil
	}
	var roots []Root
	for _, e := range entries {
		name := e.Name()
		if name != ".claude" && !strings.HasPrefix(name, ".claude-") {
			continue
		}
		p := filepath.Join(home, name)
		if !hasProjects(p) {
			continue
		}
		roots = append(roots, Root{Label: labelFor(p), Path: p})
	}
	sort.SliceStable(roots, func(i, j int) bool {
		// default dir first, then alphabetical
		if roots[i].Label == "personal" {
			return true
		}
		if roots[j].Label == "personal" {
			return false
		}
		return roots[i].Label < roots[j].Label
	})
	return roots
}

func hasProjects(root string) bool {
	st, err := os.Stat(filepath.Join(root, "projects"))
	return err == nil && st.IsDir()
}

// labelFor derives "personal" for ~/.claude and the suffix for ~/.claude-<suffix>.
func labelFor(p string) string {
	base := filepath.Base(filepath.Clean(p))
	switch {
	case base == ".claude":
		return "personal"
	case strings.HasPrefix(base, ".claude-"):
		return strings.TrimPrefix(base, ".claude-")
	}
	return strings.TrimPrefix(base, ".")
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return os.ExpandEnv(p)
}
