# Read Claude Code's transcript files directly, cached by mtime and size

Claude Code documents its transcript format as internal and unstable, and the obvious alternatives were a `SessionStart`/`Stop` hook that maintains our own index, or the `sessions-index.json` Claude writes per project. The hook needs installing in every Account and misses sessions from before the install; the index file existed in only 10 of 71 project folders on the reference machine. So unpause streams each transcript once, keeps only a handful of fields (session id, cwd, branch, timestamps, titles, last few messages), and caches the result in `~/.cache/unpause/index.json` keyed by path, mtime and size. First run on 427 MB of transcripts took about one second; cached runs take milliseconds.

## Consequences

- A Claude Code release can change field names and silently empty the list. The parser reads six fields and has fixture tests pinned to the 2.1.x format; when it breaks, fix `internal/provider/claude/scan.go` and bump `cacheVersion`.
- Live state and "cwd still exists" are computed on every run and never cached.
