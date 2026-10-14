package main

// sessionEndJobs is what the SessionEnd hook starts in the background for Session id:
// naming always, and a Recap when Recaps are on. The short delay lets Claude Code finish
// its last transcript writes first.
func sessionEndJobs(id string, recapOn bool) [][]string {
	jobs := [][]string{{"autoname", "--session", id, "--delay", "3s"}}
	if recapOn {
		jobs = append(jobs, []string{"recap", "--session", id, "--delay", "3s"})
	}
	return jobs
}
