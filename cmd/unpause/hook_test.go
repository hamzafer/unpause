package main

import (
	"reflect"
	"testing"
)

func TestSessionEndJobsStartRecapOnlyWhenEnabled(t *testing.T) {
	name := []string{"autoname", "--session", "abc", "--delay", "3s"}
	if got := sessionEndJobs("abc", false); !reflect.DeepEqual(got, [][]string{name}) {
		t.Errorf("Recaps off: only autoname, got %v", got)
	}
	want := [][]string{name, {"recap", "--session", "abc", "--delay", "3s"}}
	if got := sessionEndJobs("abc", true); !reflect.DeepEqual(got, want) {
		t.Errorf("Recaps on: autoname and recap, got %v", got)
	}
}
