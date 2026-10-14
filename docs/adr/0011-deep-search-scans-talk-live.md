# Deep search scans the talk live, with no index

Deep search finds a Session by what was said in it. The obvious alternative was a SQLite FTS index in `~/.cache/unpause`, updated as transcripts change: instant results and better ranking, but a second copy of every conversation on disk, a new dependency, and an index to keep in step with Claude Code's format. The reference machine has about 1,200 transcripts and 2.7 GB; a parallel scan of that takes around a second when the files are in the page cache, which is fast enough behind a 300 ms debounce. So the picker, `unpause search` and the MCP server's `search_sessions` all scan transcripts on each query, with one shared matcher.

The matcher searches talk only: the text you typed and Claude's replies, plus the Title. Tool output, tool calls and thinking are left out, because they make almost every query match sessions that merely ran a grep or read a file. A query's whitespace-separated terms must each appear somewhere in the Session (quotes make a phrase), the same rule as the picker's normal filter. This replaces the MCP server's earlier exact-phrase search over every record type.

## Consequences

- Search time grows with transcript volume and is slower right after a reboot, when the files are not cached. If it stops feeling instant, add an index then; this ADR is the one to supersede.
- Text that only appears in tool output (a file's contents, a command's output) can't be found. `get_session` and the picker still show it.
- `search_sessions` results are now ranked by hits and match fewer Sessions than before. Clients relying on the old behavior see a change.
