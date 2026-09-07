package opener

import "os"

// Detacher is implemented by openers that need the calling TUI to exit before they run,
// because they replace the current process.
type Detacher interface {
	Detach() bool
}

func (InPlace) Detach() bool { return true }

// Tmux replaces the process only when we have to attach to a server from outside tmux.
func (Tmux) Detach() bool { return os.Getenv("TMUX") == "" }

func (Ghostty) Detach() bool { return false }

// NeedsDetach reports whether o must run after the TUI has exited.
func NeedsDetach(o Opener) bool {
	if d, ok := o.(Detacher); ok {
		return d.Detach()
	}
	return false
}
