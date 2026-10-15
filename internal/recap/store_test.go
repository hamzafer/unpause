package recap

import (
	"fmt"
	"github.com/hamzafer/unpause/internal/session"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Two writers (hook children, or a hook child and a backfill) must not lose each other's
// Recap: Put's read-modify-write runs under a lock.
func TestConcurrentPutsKeepEveryRecap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unpause", "recaps.json")
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Put(path, fmt.Sprintf("s%02d", i), Recap{Goal: "g", Done: "d", Next: "n", Size: int64(i)})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != n {
		t.Errorf("store has %d Recaps after %d concurrent Puts, want all of them", len(st), n)
	}
}

func TestPathSitsNextToTheIndex(t *testing.T) {
	if got := Path("/c/unpause/index.json"); got != "/c/unpause/recaps.json" {
		t.Errorf("Path = %s", got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unpause", "recaps.json")
	got, err := Load(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("a missing file is an empty store, got %v, %v", got, err)
	}
	at := time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)
	a := Recap{Goal: "g1", Done: "d1", Next: "n1", Size: 1234, At: at}
	b := Recap{Goal: "g2", Done: "d2", Next: "n2", Size: 99, At: at}
	if err := Put(path, "aaa", a); err != nil {
		t.Fatal(err)
	}
	if err := Put(path, "bbb", b); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["aaa"] != a || got["bbb"] != b {
		t.Errorf("round trip: %+v", got)
	}
	a.Goal = "g1 again"
	if err := Put(path, "aaa", a); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got["aaa"].Goal != "g1 again" || got["bbb"] != b {
		t.Errorf("a new Recap replaces only its own Session's: %+v", got)
	}
	// Only the store and its lock file, which stays: removing it would race a waiting writer.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 || entries[0].Name() != "recaps.json" || entries[1].Name() != "recaps.json.lock" {
		t.Errorf("no temp files should be left behind: %v", entries)
	}
}

func TestLoadRejectsACorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recaps.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a corrupt file should be an error")
	}
}

// A recaps.json holding JSON null decodes to a nil map; Put must still store into it.
func TestPutOverANullStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recaps.json")
	if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, err := Load(path); err != nil || st == nil {
		t.Fatalf("a null store must load as an empty one, got %v, %v", st, err)
	}
	r := Recap{Goal: "g", Done: "d", Next: "n", Size: 1}
	if err := Put(path, "aaa", r); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got["aaa"] != r {
		t.Errorf("store = %+v", got)
	}
}

func TestTwoTranscriptsSharingAnIDKeepTheirOwnRecaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recaps.json")
	a := &session.Session{ID: "same", Path: "/w/a.jsonl"}
	b := &session.Session{ID: "same", Path: "/p/b.jsonl"}
	if err := Put(path, Key(a), Recap{Goal: "from a"}); err != nil {
		t.Fatal(err)
	}
	if err := Put(path, Key(b), Recap{Goal: "from b"}); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if ra, _ := st.Get(a); ra.Goal != "from a" {
		t.Errorf("a = %+v", ra)
	}
	if rb, _ := st.Get(b); rb.Goal != "from b" {
		t.Errorf("b = %+v", rb)
	}
}

func TestMigrateMovesIDKeysToPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recaps.json")
	old := `{"uniq": {"goal": "unique one"}, "dup": {"goal": "ambiguous"}, "/x/kept.jsonl": {"goal": "already a path"}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	uniq := &session.Session{ID: "uniq", Path: "/x/uniq.jsonl", Messages: 1}
	d1 := &session.Session{ID: "dup", Path: "/x/d1.jsonl", Messages: 1}
	d2 := &session.Session{ID: "dup", Path: "/y/d2.jsonl", Messages: 1}
	if err := Migrate(path, []*session.Session{uniq, d1, d2}); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := st.Get(uniq); !ok || r.Goal != "unique one" {
		t.Errorf("unique id must move to its path: %v", st)
	}
	if _, ok := st.Get(d1); ok {
		t.Errorf("a shared id can't be placed and must be dropped: %v", st)
	}
	if len(st) != 2 || st["/x/kept.jsonl"].Goal != "already a path" {
		t.Errorf("store = %v", st)
	}
	before, _ := os.Stat(path)
	if err := Migrate(path, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(path); !after.ModTime().Equal(before.ModTime()) {
		t.Error("nothing to move must not rewrite the store")
	}
}

func TestMigrateIgnoresSubagentsAndKeepsUnmatchedIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recaps.json")
	old := `{"parent": {"goal": "the parent"}, "elsewhere": {"goal": "not indexed right now"}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := &session.Session{ID: "parent", Path: "/x/parent.jsonl", Messages: 4}
	sub := &session.Session{ID: "parent", Path: "/x/parent/subagents/a.jsonl", Messages: 2, Sidechain: true}
	if err := Migrate(path, []*session.Session{parent, sub}); err != nil {
		t.Fatal(err)
	}
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := st.Get(parent); !ok || r.Goal != "the parent" {
		t.Errorf("a subagent sharing the id must not make it ambiguous: %v", st)
	}
	if r, ok := st["elsewhere"]; !ok || r.Goal != "not indexed right now" {
		t.Errorf("an id no Session has must be kept: %v", st)
	}
}
