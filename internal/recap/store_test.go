package recap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
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
