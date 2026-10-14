# Recaps live in unpause's cache, not the Transcript

A Recap is three lines (goal, done, next) a small model writes about a Session. Names are written into the Transcript as a `/rename` record (ADR 0005) because Claude Code reads them too; a Recap means nothing to Claude Code, changes every time the Session grows, and would be a second kind of write into another tool's files. So Recaps are stored in `~/.cache/unpause/recaps.json`, keyed by Session id with the transcript size each was made from, and the Transcript is never touched.

They are generated the same way as names (ADR 0008): `claude -p --model haiku` under the Session's own Account, from the SessionEnd hook in a detached child, or by a manual backfill. Generating them while the picker is open was rejected: a Haiku call takes seconds, and scrolling would fire dozens of them.

Recaps are off unless `[recap] enabled = true`, because each one sends part of a Session's talk to the model.

## Consequences

- Each Recap costs one small Haiku call against the Account's plan, at every Session end while enabled.
- A Recap made before the Session grew is outdated until it is regenerated; the picker marks it instead of hiding it.
- Clearing the cache loses the Recaps, nothing else; backfill makes them again.
