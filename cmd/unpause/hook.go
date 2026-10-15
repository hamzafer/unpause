package main

import (
	"path/filepath"
	"time"

	"github.com/hamzafer/unpause/internal/session"
)

// hookDelay is how long a SessionEnd job waits before starting, so Claude Code can finish
// its last transcript writes first.
const hookDelay = 3 * time.Second

// sessionEndJobs is what the SessionEnd hook starts in the background for Session id:
// naming always, and a Recap when Recaps are on. The Recap job also gets the transcript
// that ended, because two Transcripts can share a Session id.
func sessionEndJobs(id, transcript string, recapOn bool) [][]string {
	delay := []string{"--delay", hookDelay.String()}
	jobs := [][]string{append([]string{"autoname", "--session", id}, delay...)}
	if recapOn {
		job := []string{"recap", "--session", id}
		if transcript != "" {
			job = append(job, "--transcript", transcript)
		}
		jobs = append(jobs, append(job, delay...))
	}
	return jobs
}

// recapTarget picks the Session `unpause recap --session` works on: the one whose
// Transcript is transcript when that is given and found, else the usual id-or-name match.
func recapTarget(sessions []*session.Session, only, transcript string) *session.Session {
	if transcript != "" {
		want := filepath.Clean(transcript)
		for _, s := range sessions {
			if filepath.Clean(s.Path) == want {
				return s
			}
		}
	}
	return find(sessions, only)
}
