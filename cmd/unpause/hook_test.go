package main

import (
	"reflect"
	"testing"

	"github.com/hamzafer/unpause/internal/session"
)

func TestSessionEndJobsStartRecapOnlyWhenEnabled(t *testing.T) {
	name := []string{"autoname", "--session", "abc", "--delay", hookDelay.String()}
	if got := sessionEndJobs("abc", "/t/abc.jsonl", false); !reflect.DeepEqual(got, [][]string{name}) {
		t.Errorf("Recaps off: only autoname, got %v", got)
	}
	want := [][]string{name, {"recap", "--session", "abc", "--transcript", "/t/abc.jsonl", "--delay", hookDelay.String()}}
	if got := sessionEndJobs("abc", "/t/abc.jsonl", true); !reflect.DeepEqual(got, want) {
		t.Errorf("Recaps on: autoname and recap, got %v", got)
	}
}

func TestRecapTargetPrefersTheTranscriptThatEnded(t *testing.T) {
	a := &session.Session{ID: "same-id", Path: "/w/a.jsonl"}
	b := &session.Session{ID: "same-id", Path: "/p/b.jsonl"}
	all := []*session.Session{a, b}
	if got := recapTarget(all, "same-id", "/p/b.jsonl"); got != b {
		t.Errorf("got %v, want the transcript the hook named", got)
	}
	if got := recapTarget(all, "same-id", ""); got == nil {
		t.Error("without a transcript, the id match still works")
	}
	if got := recapTarget(all, "same-id", "/gone.jsonl"); got == nil {
		t.Error("an unknown transcript falls back to the id match")
	}
}
