package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hamzafer/unpause/internal/session"
)

type liveFile struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	Name       string `json:"name"`
	NameSource string `json:"nameSource"`
	Status     string `json:"status"`
	Kind       string `json:"kind"`
}

// LiveSessions reads <root>/sessions/*.json and returns the ones whose process is still alive, keyed by session id.
func LiveSessions(root string) map[string]session.Live {
	out := map[string]session.Live{}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, "sessions", e.Name()))
		if err != nil {
			continue
		}
		var lf liveFile
		if json.Unmarshal(b, &lf) != nil || lf.PID == 0 || lf.SessionID == "" {
			continue
		}
		if !alive(lf.PID) {
			continue
		}
		name := ""
		if lf.NameSource == "custom" {
			name = lf.Name
		}
		out[lf.SessionID] = session.Live{PID: lf.PID, Status: lf.Status, Name: name}
	}
	return out
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
