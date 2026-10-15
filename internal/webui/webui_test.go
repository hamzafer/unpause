package webui

import (
	"io/fs"
	"testing"
)

func TestFSHasIndex(t *testing.T) {
	b, err := fs.ReadFile(FS(), "index.html")
	if err != nil || len(b) == 0 {
		t.Fatalf("index.html: %v (%d bytes)", err, len(b))
	}
}
