package claude

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadCowork(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "local_1", ".claude")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(dir, "local_1.json"), []byte(`{"sessionId":"local_1","cliSessionId":"abc","title":" Build metrics dashboard ","createdAt":1769460665209,"lastActivityAt":1769460775175,"isArchived":false,`+
		`"userSelectedFolders":["/Users/x/Downloads/foo",""," /Users/x/work "],"cwd":"/sessions/determined-modest-edison"}`), 0o644)
	got := ReadCowork(root)
	want := CoworkMeta{
		Title:   "Build metrics dashboard",
		Folders: []string{"/Users/x/Downloads/foo", "/Users/x/work"},
		CWD:     "/sessions/determined-modest-edison",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadCowork = %+v, want %+v", got, want)
	}
	if got := ReadCowork(filepath.Join(dir, "missing", ".claude")); !reflect.DeepEqual(got, CoworkMeta{}) {
		t.Errorf("missing metadata should give nothing, got %+v", got)
	}
}
