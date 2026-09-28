package autoname

import "os/exec"

// detach is a no-op on Windows; the child is still started without waiting for it.
func detach(cmd *exec.Cmd) {}
