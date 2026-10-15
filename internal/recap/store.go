package recap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzafer/unpause/internal/session"
)

// Store is every Recap, keyed by Key: the Transcript's path. Not the Session id, because a
// Session resumed in another folder or Account keeps its id in a second Transcript.
type Store map[string]Recap

// Key is where s's Recap lives in a Store.
func Key(s *session.Session) string { return s.Path }

// Get returns s's Recap.
func (st Store) Get(s *session.Session) (Recap, bool) {
	r, ok := st[Key(s)]
	return r, ok
}

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
func Put(path, key string, r Recap) error {
	return update(path, func(st Store) bool { st[key] = r; return true })
}

// Migrate moves Recaps that v0.8 stored under a Session id to their Transcript's path.
// An id more than one Transcript shares can't be placed, so its Recap is dropped and made
// again by the next backfill. It writes only when there was something to move.
func Migrate(path string, sessions []*session.Session) error {
	if _, err := os.Stat(path); err != nil {
		return nil // no store yet, nothing to move
	}
	byID := map[string][]*session.Session{}
	for _, s := range sessions {
		byID[s.ID] = append(byID[s.ID], s)
	}
	return update(path, func(st Store) bool {
		changed := false
		for k, r := range st {
			if strings.ContainsAny(k, `/\`) {
				continue // already a path
			}
			if ss := byID[k]; len(ss) == 1 {
				if _, taken := st[Key(ss[0])]; !taken {
					st[Key(ss[0])] = r
				}
			}
			delete(st, k)
			changed = true
		}
		return changed
	})
}

// update runs change on the store under the lock and writes the result when change
// reports it changed something.
func update(path string, change func(Store) bool) error {
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
	if !change(st) {
		return nil
	}
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
