package claude

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// Rename appends the same record Claude Code writes for /rename, so the name shows up in
// Claude's own picker too. Append-only: existing lines are never touched.
func Rename(transcriptPath, sessionID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is empty")
	}
	rec, err := json.Marshal(map[string]string{
		"type":        "custom-title",
		"customTitle": name,
		"sessionId":   sessionID,
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Make sure we start on a fresh line even if the file lacks a trailing newline.
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		tail := make([]byte, 1)
		if rf, err := os.Open(transcriptPath); err == nil {
			if _, err := rf.ReadAt(tail, st.Size()-1); err == nil && tail[0] != '\n' {
				_, _ = f.Write([]byte("\n"))
			}
			rf.Close()
		}
	}
	_, err = f.Write(append(rec, '\n'))
	return err
}
