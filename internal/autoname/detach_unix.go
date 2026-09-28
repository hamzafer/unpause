//go:build !windows

package autoname

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in its own session so it outlives the Claude Code process that ran the hook.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
