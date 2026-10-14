package recap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

// onDisk makes a Session whose transcript is size bytes, last active at the given hour.
func onDisk(t *testing.T, id string, size, hour int) *session.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".jsonl")
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return &session.Session{ID: id, Path: path, Messages: 2,
		LastActive: time.Date(2026, 10, 15, hour, 0, 0, 0, time.UTC)}
}

func ids(ss []*session.Session) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestOutdatedWhenTheTranscriptGrew(t *testing.T) {
	s := onDisk(t, "a", 100, 1)
	if Outdated(Recap{Size: 100}, s) {
		t.Error("same size is not outdated")
	}
	if !Outdated(Recap{Size: 60}, s) {
		t.Error("a transcript that grew makes the Recap outdated")
	}
	gone := &session.Session{ID: "g", Path: filepath.Join(t.TempDir(), "missing.jsonl")}
	if Outdated(Recap{Size: 10}, gone) {
		t.Error("a missing transcript can't be recapped again, so it is not outdated")
	}
}

func TestTodoPicksMissingAndOutdatedNewestFirst(t *testing.T) {
	missing := onDisk(t, "missing", 50, 3)
	outdated := onDisk(t, "outdated", 80, 5)
	current := onDisk(t, "current", 40, 9)
	newest := onDisk(t, "newest", 10, 10)
	live := onDisk(t, "live", 10, 11)
	live.Live = &session.Live{PID: 1}
	cowork := onDisk(t, "cowork", 10, 12)
	cowork.Cowork = true
	empty := onDisk(t, "empty", 10, 13)
	empty.Messages = 0
	side := onDisk(t, "side", 10, 14)
	side.Sidechain = true
	st := Store{"outdated": {Size: 30}, "current": {Size: 40}}

	all := []*session.Session{missing, outdated, current, newest, live, cowork, empty, side}
	got := ids(Todo(all, st, 0))
	want := []string{"newest", "outdated", "missing"}
	if len(got) != len(want) {
		t.Fatalf("Todo = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Todo = %v, want %v", got, want)
		}
	}
	if got := ids(Todo(all, st, 2)); len(got) != 2 || got[0] != "newest" || got[1] != "outdated" {
		t.Errorf("limit 2 = %v", got)
	}
	if got := Todo(all, st, 0); len(all) != 8 || all[0].ID != "missing" || len(got) != 3 {
		t.Error("Todo must not reorder the caller's slice")
	}
}

func TestCountHaveAndOutdated(t *testing.T) {
	a := onDisk(t, "a", 50, 1)
	b := onDisk(t, "b", 50, 2)
	c := onDisk(t, "c", 50, 3)
	have, outdated := Count([]*session.Session{a, b, c}, Store{"a": {Size: 50}, "b": {Size: 20}, "gone": {Size: 1}})
	if have != 2 || outdated != 1 {
		t.Errorf("have %d, outdated %d; want 2, 1", have, outdated)
	}
}

func TestBackfillStoresEachAndReportsErrors(t *testing.T) {
	good := transcriptSession(t)
	bad := transcriptSession(t)
	bad.ID = "bad"
	boom := errors.New("claude -p: exit 1")
	model := func(_ context.Context, s *session.Session, _ string) (string, error) {
		if s.ID == "bad" {
			return "", boom
		}
		return goodReply, nil
	}
	stored := map[string]Recap{}
	put := func(id string, r Recap) error { stored[id] = r; return nil }
	var reported []Result
	Backfill(context.Background(), []*session.Session{good, bad}, model, put, func(r Result) { reported = append(reported, r) })
	if len(reported) != 2 || reported[0].Err != nil || !errors.Is(reported[1].Err, boom) {
		t.Fatalf("reported = %+v", reported)
	}
	if reported[0].Recap.Goal != "set up the tunnel" {
		t.Errorf("first recap = %+v", reported[0].Recap)
	}
	if _, ok := stored[good.ID]; !ok || len(stored) != 1 {
		t.Errorf("stored = %v", stored)
	}
}

func TestBackfillReportsAStoreError(t *testing.T) {
	s := transcriptSession(t)
	full := errors.New("disk full")
	var got error
	Backfill(context.Background(), []*session.Session{s},
		func(context.Context, *session.Session, string) (string, error) { return goodReply, nil },
		func(string, Recap) error { return full },
		func(r Result) { got = r.Err })
	if !errors.Is(got, full) {
		t.Errorf("err = %v", got)
	}
}
