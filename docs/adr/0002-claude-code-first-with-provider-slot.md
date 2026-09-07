# Claude Code only in v1, behind a provider slot

The promise is "every AI coding session on your machine", but every agent stores sessions in its own undocumented format that shifts with releases. v1 parses Claude Code only. The internal model carries a `Provider` field and the parser lives in `internal/provider/claude`, so Codex or OpenCode can be added as siblings without touching the list, cache or opener. The README says so, and the name deliberately does not contain "claude".
