package opener

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Warp opens a new tab in the running Warp instance via a Tab Config: a TOML file
// under ~/.warp/tab_configs/ triggered by the warp://tab_config/<name> URI, matched
// case-insensitively against the file's stem. unpause reuses one file (overwritten on
// every launch) so it doesn't litter the tab_configs directory.
// https://docs.warp.dev/terminal/windows/tab-configs
type Warp struct{}

func (Warp) Name() string { return "warp-tab" }

const warpTabConfigName = "unpause-launch"

func (Warp) Open(l Launch) error {
	dir, err := warpTabConfigsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := fmt.Sprintf("%s -lic %s", shell, shellQuote(l.ShellCommand()))

	toml := fmt.Sprintf(`name = %s

[[panes]]
id = "main"
type = "terminal"
directory = %s
commands = [%s]
`, tomlString(l.Title), tomlString(l.CWD), tomlString(cmd))

	path := filepath.Join(dir, warpTabConfigName+".toml")
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		return err
	}

	out, err := exec.Command("open", "warp://tab_config/"+warpTabConfigName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("warp: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (Warp) Detach() bool { return false }

func warpTabConfigsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".warp", "tab_configs"), nil
}

// tomlString renders a Go string as a quoted TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
