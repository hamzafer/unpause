package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoworkTitle(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "local_1", ".claude")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(dir, "local_1.json"), []byte(`{"sessionId":"local_1","cliSessionId":"abc","title":"Build metrics dashboard","createdAt":1769460665209,"lastActivityAt":1769460775175,"isArchived":false}`), 0o644)
	if got := CoworkTitle(root); got != "Build metrics dashboard" {
		t.Errorf("title = %q", got)
	}
	if got := CoworkTitle(filepath.Join(dir, "missing", ".claude")); got != "" {
		t.Errorf("missing metadata should give no title, got %q", got)
	}
}
