package opener

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Ghostty opens a new tab in the frontmost Ghostty window through its AppleScript dictionary
// (Ghostty >= 1.3 ships Ghostty.sdef with "new tab" and "surface configuration").
type Ghostty struct{}

func (Ghostty) Name() string { return "ghostty-tab" }

func (Ghostty) Open(l Launch) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	// Run through a login+interactive shell so PATH, aliases and prompt hooks match a normal tab.
	cmd := fmt.Sprintf("%s -lic %s", shell, shellQuote(l.ShellCommand()))

	script := fmt.Sprintf(`tell application "Ghostty"
	set cfg to new surface configuration
	set initial working directory of cfg to %s
	set command of cfg to %s
	set wait after command of cfg to false
	set environment variables of cfg to {%s}
	if (count of windows) is 0 then
		new window with configuration cfg
	else
		new tab in front window with configuration cfg
	end if
	activate
end tell`,
		asQuote(l.CWD), asQuote(cmd), asQuote("CLAUDE_CONFIG_DIR="+l.ConfigDir))

	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ghostty: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func asQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
