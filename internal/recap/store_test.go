package recap

import (
	"fmt"
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
