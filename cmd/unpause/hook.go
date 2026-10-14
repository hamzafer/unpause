package main

import "time"

// hookDelay is how long a SessionEnd job waits before starting, so Claude Code can finish
// its last transcript writes first.
const hookDelay = 3 * time.Second

// sessionEndJobs is what the SessionEnd hook starts in the background for Session id:
// naming always, and a Recap when Recaps are on.
func sessionEndJobs(id string, recapOn bool) [][]string {
	delay := []string{"--delay", hookDelay.String()}
	jobs := [][]string{append([]string{"autoname", "--session", id}, delay...)}
	if recapOn {
		jobs = append(jobs, append([]string{"recap", "--session", id}, delay...))
	}
	return jobs
}
