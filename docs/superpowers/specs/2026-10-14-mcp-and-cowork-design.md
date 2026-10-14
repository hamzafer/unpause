# unpause mcp and Cowork sessions

Decided in a grill session on 2026-10-14. Two vertical slices; they can be built in parallel.

## Goal

Let ChatGPT (web) read every Session on this machine: search them, list them, open one and read it in full. unpause already knows every Account, title and live state, so it serves them itself as a read-only MCP server. Cowork sessions from Claude Desktop join the list as their own Account.

## Slice 1: Cowork Account

Claude Desktop's Cowork mode runs Claude Code inside a VM and keeps one config root per Session on the host:

```
~/Library/Application Support/Claude/local-agent-mode-sessions/<a>/<b>/local_<x>.json          metadata
~/Library/Application Support/Claude/local-agent-mode-sessions/<a>/<b>/local_<x>/.claude/projects/<enc-cwd>/<cli-session-id>.jsonl
```

The metadata file has `title`, `cliSessionId`, `createdAt`, `lastActivityAt` (ms epoch), `isArchived`, `initialMessage`, `cwd`. The transcript is standard Claude Code format, so `claude.ScanFile` parses it unchanged.

- `config.ResolveRoots` adds every `local_<x>/.claude` that has `projects/` as a Root labelled `cowork` (macOS only; the folder simply doesn't exist elsewhere). `no_auto_detect` turns it off with the rest of auto-detection. A Root gets a `Cowork bool`.
- Title: the metadata `title` fills the Session's title when the transcript has no `custom-title` (it wins over the transcript's `ai-title`, because it is what Claude Desktop shows). Stored in a new `DesktopTitle` field, so `Title()` order is CustomTitle, DesktopTitle, AITitle, FirstPrompt. Cached with the session; bump `cacheVersion`.
- A Session gets `Cowork bool` (from its Root). Cowork sessions are read-only to unpause: the picker and `unpause open` refuse with "Cowork session: open it in Claude Desktop", `rename` refuses, `autoname` skips them. They ran inside the VM; resuming them on the host would write into Claude Desktop's folder.
- They show in `unpause`, `unpause list`, `--json` and the MCP like any other Session. Subagent transcripts under them are ignored as today.
- `unpause doctor` lists the cowork roots as one summary line (`cowork  N roots, M transcripts`), not one line per root.

Tests: fixture tree with two Cowork sessions (one titled, one not) under a temp HOME; ResolveRoots finds them, titles come out right, open/rename refuse.

## Slice 2: `unpause mcp`

A stdio MCP server, `unpause mcp`, built on `github.com/modelcontextprotocol/go-sdk` v1.8.0. Read-only: it never writes to a transcript, the cache aside. Logs go to stderr only. Every tool has `readOnlyHint: true`.

It loads sessions with the same `loadAll(true)` + default filter as `unpause list` (non-empty, not sidechain), on every call, so results are fresh.

### Exclude

`~/.config/unpause/config.toml`:

```toml
[mcp]
exclude = ["~/developer/work/*", "*/secret-repo"]
```

Globs (`~` expanded, `path.Match` syntax) matched against a Session's cwd. A pattern also matches every folder below a match. Excluded Sessions are invisible to all three tools (get_session says "not found"). The picker and `unpause list` are unaffected.

### Tools

`list_sessions`
- in: `account?` (exact label), `project?` (case-insensitive substring of cwd), `live?` (bool), `limit` (default 20, max 100), `offset` (default 0)
- out: `{total, offset, next_offset?, sessions: [Summary]}`, newest first
- Summary: `id, title, account, project (repo folder name), cwd, branch, created, last_active, messages, live ("busy"/"idle"/omitted), cowork?`

`search_sessions`
- in: `query` (required), `account?`, `project?`, `limit` (default 10, max 50), `offset`
- Plain case-insensitive substring match over the title and the full text of every user and assistant record (text, tool calls, tool results, thinking text). No ranking: matching Sessions newest first.
- out: `{total, offset, next_offset?, results: [Summary + hits (count) + snippets (up to 3, ~200 chars around a match)]}`
- Must finish in well under 120 s (tunnel deadline) on the reference machine (~2.6 GB, ~1,200 transcripts): scan files in parallel, pre-filter raw lines by lowercase bytes before decoding JSON.

`get_session`
- in: `id` (id, id prefix, or Name, same lookup as `unpause open`), `offset` (message index, default 0), `limit` (default 50, max 200), `max_chars` (per content block, default 20000)
- out: `{session: Summary, total, offset, next_offset?, messages: [{index, role, at, blocks: [{type, text?, name?, input?, is_error?}]}]}`
- Messages are every `user` and `assistant` record with content, in file order, raw: tool_use blocks carry name and input, tool_result blocks carry their text, thinking blocks carry their text. Blocks longer than `max_chars` are cut with a `[… N more chars]` marker.

### ChatGPT

README section "Use from ChatGPT": OpenAI Secure MCP Tunnel (outbound only, nothing listens on the Mac):

```bash
tunnel-client init --sample sample_mcp_stdio_local --profile unpause \
  --tunnel-id tunnel_... --mcp-command "unpause mcp"
tunnel-client run --profile unpause
```

Then ChatGPT → Plugins → add custom MCP server, Connection: Tunnel. State plainly that full transcripts, tool output included, go to OpenAI, and point at `[mcp] exclude`.

Tests: in-memory MCP client/server over the SDK's in-memory transport against a fixture root: each tool's paging, filters, exclude, id lookup, block truncation, read-only (transcript bytes unchanged).

## Out of scope

claude.ai chat export import, a ChatGPT widget, search inside the picker, an HTTP transport.
