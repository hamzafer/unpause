package recap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamzafer/unpause/internal/session"
)

func TestOutdatedWhenTheTranscriptChangedSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &session.Session{ID: "x", Path: path}
	if Outdated(Recap{Size: 10}, s) {
		t.Error("a Recap made from the current transcript is not outdated")
	}
	if !Outdated(Recap{Size: 4}, s) {
		t.Error("a Recap made from a smaller transcript is outdated")
	}
	if Outdated(Recap{Size: 4}, &session.Session{ID: "y", Path: filepath.Join(t.TempDir(), "gone.jsonl")}) {
		t.Error("a missing transcript can't say the Recap is outdated")
	}
}
