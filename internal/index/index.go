// Package index builds the cross-root session list, caching per-file parse results by mtime and size.
package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/hamzafer/unpause/internal/config"
	"github.com/hamzafer/unpause/internal/provider/claude"
	"github.com/hamzafer/unpause/internal/session"
)

const cacheVersion = 3

type entry struct {
	ModTime int64            `json:"mtime"`
	Size    int64            `json:"size"`
	Session *session.Session `json:"session"`
}

type cacheFile struct {
	Version int              `json:"version"`
	Entries map[string]entry `json:"entries"` // keyed by transcript path
}

// Progress is called as files are (re)parsed; n is done, total is the number needing a parse.
type Progress func(n, total int)

// Build returns every session across the given roots, newest first.
func Build(roots []config.Root, progress Progress) ([]*session.Session, error) {
	cache := load()
	fresh := cacheFile{Version: cacheVersion, Entries: map[string]entry{}}

	type job struct {
		t claude.Transcript
	}
	var jobs []job
	var sessions []*session.Session
	var mu sync.Mutex

	for _, r := range roots {
		ts, err := claude.ListTranscripts(r)
		if err != nil {
			continue
		}
		for _, t := range ts {
			if e, ok := cache.Entries[t.Path]; ok && e.ModTime == t.ModTime && e.Size == t.Size && e.Session != nil {
				e.Session.Root = r.Path
				e.Session.Account = r.Label
				fresh.Entries[t.Path] = e
				sessions = append(sessions, e.Session)
				continue
			}
			jobs = append(jobs, job{t})
		}
	}

	total := len(jobs)
	done := 0
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			s, err := claude.ScanFile(j.t.Path)
			mu.Lock()
			defer mu.Unlock()
			done++
			if progress != nil {
				progress(done, total)
			}
			if err != nil {
				return
			}
			s.Root = j.t.Root.Path
			s.Account = j.t.Root.Label
			fresh.Entries[j.t.Path] = entry{ModTime: j.t.ModTime, Size: j.t.Size, Session: s}
			sessions = append(sessions, s)
		}(j)
	}
	wg.Wait()

	if total > 0 || len(fresh.Entries) != len(cache.Entries) {
		_ = save(fresh)
	}

	// Attach live state and cwd existence; never cached.
	for _, r := range roots {
		live := claude.LiveSessions(r.Path)
		for _, s := range sessions {
			if s.Root != r.Path {
				continue
			}
			if l, ok := live[s.ID]; ok {
				l := l
				s.Live = &l
			}
		}
	}
	for _, s := range sessions {
		if s.CWD != "" {
			if _, err := os.Stat(s.CWD); err != nil {
				s.CWDMissing = true
			}
		}
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})
	return sessions, nil
}

// Invalidate drops one path from the cache so the next Build re-parses it.
func Invalidate(path string) {
	c := load()
	delete(c.Entries, path)
	_ = save(c)
}

func load() cacheFile {
	c := cacheFile{Version: cacheVersion, Entries: map[string]entry{}}
	b, err := os.ReadFile(config.CachePath())
	if err != nil {
		return c
	}
	var on cacheFile
	if json.Unmarshal(b, &on) != nil || on.Version != cacheVersion || on.Entries == nil {
		return c
	}
	return on
}

func save(c cacheFile) error {
	p := config.CachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
