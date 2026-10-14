package recap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Store is every Recap, keyed by Session id.
type Store map[string]Recap

// Path is where Recaps are kept: recaps.json next to the index cache file.
func Path(cacheFile string) string {
	return filepath.Join(filepath.Dir(cacheFile), "recaps.json")
}

// Load reads the store. A missing file, or one holding JSON null, is an empty store.
func Load(path string) (Store, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	st := Store{}
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if st == nil { // the file held JSON null
		st = Store{}
	}
	return st, nil
}

// Put stores r for Session id, keeping every other Session's Recap. A corrupt file is
// started over: it is a cache, and losing it loses nothing but Recaps. The file is
// replaced in one rename, so a reader never sees half of it. The read-modify-write runs
// under a lock on recaps.json.lock, so two writers (two hook children, or a hook child and
// a backfill) can't lose each other's Recap.
func Put(path, id string, r Recap) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close() // closing releases the lock
	if err := lockFile(lock); err != nil {
		return err
	}
	st, err := Load(path)
	if err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		if !errors.As(err, &syn) && !errors.As(err, &typ) {
			return err
		}
		st = Store{}
	}
	st[id] = r
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recaps-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
