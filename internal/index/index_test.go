package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/session"
)

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAddsCoworkFoldersToDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	id := "c0c0c0c0-1111-2222-3333-444444444444"
	root := filepath.Join(dir, "local_aaa", ".claude")
	rec := func(cwd string) string {
		return `{"type":"user","message":{"role":"user","content":"hi"},"timestamp":"2026-09-28T10:00:00Z","cwd":"` + cwd + `","sessionId":"` + id + `"}` + "\n"
	}
	writeFile(t, filepath.Join(root, "projects", "-sessions-x", id+".jsonl"), rec("/sessions/x")+rec("/sessions/x/sub"))
	writeFile(t, filepath.Join(dir, "local_aaa.json"), `{"title":"Charts","userSelectedFolders":["/Users/me/Downloads/foo"],"cwd":"/sessions/x"}`)
	roots := []config.Root{{Label: config.CoworkLabel, Path: root, Cowork: true}}

	for run := 0; run < 2; run++ { // second run comes from the cache
		ss, err := Build(roots, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ss) != 1 {
			t.Fatalf("run %d: %d sessions", run, len(ss))
		}
		if got := strings.Join(ss[0].Dirs, ","); got != "/sessions/x,/sessions/x/sub,/Users/me/Downloads/foo" {
			t.Errorf("run %d: dirs = %q", run, got)
		}
		if ss[0].DesktopTitle != "Charts" {
			t.Errorf("run %d: desktop title = %q", run, ss[0].DesktopTitle)
		}
	}
	// Cowork folders and the Desktop title are read every run, never cached.
	b, err := os.ReadFile(config.CachePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Downloads/foo") || strings.Contains(string(b), "Charts") {
		t.Errorf("cache holds Cowork metadata: %s", b)
	}
}

func TestConcurrentSavesNeverCorruptTheCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := cacheFile{Version: cacheVersion, Entries: map[string]entry{}}
			for j := 0; j < 200; j++ {
				c.Entries[filepath.Join("/p", strings.Repeat("x", i), string(rune('a'+j%26)), strings.Repeat("y", j))] = entry{Session: &session.Session{ID: "s"}}
			}
			for k := 0; k < 5; k++ {
				if err := save(c); err != nil {
					t.Errorf("save: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
	b, err := os.ReadFile(config.CachePath())
	if err != nil {
		t.Fatal(err)
	}
	var c cacheFile
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("cache is not valid JSON after concurrent saves: %v", err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(config.CachePath()), "*tmp*"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}
