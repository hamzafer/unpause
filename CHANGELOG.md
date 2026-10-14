# Changelog

## v0.7.0, 2026-10-15

- Deep search in the picker: `^s` switches the filter box to searching inside what was said, with the same rules as `unpause search`. It starts 300 ms after you stop typing, shows scan progress in the header, lists matching sessions ranked with a `12×` match count, and previews up to 3 snippets with your words highlighted. Whatever you'd typed in the filter becomes the first query; `^s` or `esc` goes back.
- `unpause search <words>` finds sessions by what was said in them: your prompts, Claude's replies, and the session's name or generated title. Every word has to appear somewhere in the session, `"double quotes"` keep a phrase together, and results are ranked by match count with the first snippet under each line. `--json`, `--all` and `--limit N` (default 20, 0 for all) work as you'd expect.
- `search_sessions` in `unpause mcp` uses the same search. It no longer looks at tool calls, tool output or thinking, needs every word instead of one exact string, and ranks by match count instead of newest first, so it finds fewer, better sessions.

## v0.6.0, 2026-10-14

- Claude Desktop's Cowork sessions are listed under a read-only `cowork` account, titled the way Claude Desktop shows them. The picker, `unpause open` and `unpause rename` refuse them with a pointer to Claude Desktop, `autoname` skips them, and `unpause doctor` sums them up in one line.
- `unpause mcp` is a read-only MCP server on stdio with three tools, `list_sessions`, `search_sessions` and `get_session`, so ChatGPT can read your sessions through OpenAI's Secure MCP Tunnel. `[mcp] exclude` globs in the config hide folders from it: a session is hidden if any folder it worked in matches, Cowork sessions included through the folders you gave them in Claude Desktop. A broken pattern stops the server from starting. Search also matches tool names. Setup is in the README under "Use from ChatGPT".

## v0.5.0, 2026-10-14

- Retired worktrees: a session whose Claude Code worktree (`<repo>/.claude/worktrees/<name>`) was removed while its repo still exists gets a dim `⎇` instead of the orphan `!`, and opens in the repo (or the same subfolder of it, when that exists) instead of `~`. `unpause list` shows ` (worktree retired)` for these; the JSON keeps `cwd_missing` and adds `worktree_root`.
- `unpause open` starts orphans in `~` and retired worktrees in their repo, like the picker. Before, it tried the missing folder.

## v0.4.1, 2026-09-28

- `unpause autoname --dry-run` saves its proposals, and `unpause autoname --apply` writes exactly those names without asking Haiku again. Before, approving a dry run and then running for real got freshly worded names.
- Fix: a Haiku reply that asks a question or runs past 8 words is rejected instead of written as the name.

## v0.4.0, 2026-09-28

- `unpause autoname` names sessions that have neither a name you gave nor a Claude title, using Claude Haiku through `claude -p` under each session's own account. `--dry-run` shows the proposed names without writing. An account whose login has expired fails once and its remaining sessions are skipped.
- `unpause hook session-end` is a `SessionEnd` hook entry point: it starts a detached `autoname` for the session that just ended and returns immediately. Setup is in the README.
- Fix: a CLI test assumed a real Claude account existed on the machine.

## v0.3.0, 2026-09-07

- Warp gets a real tab opener: `--open warp` (and auto-detection via `TERM_PROGRAM`) opens a new tab in the running Warp window through a reusable Tab Config, instead of falling back to in-place.
- `unpause open <id> --print --json` emits the launch as structured JSON (claude, session id, cwd, config dir, args, fork, and the shell one-liner) for scripts that don't want to parse a shell string.
- Shell completions (bash, zsh, fish) are generated at release time and installed automatically by the Homebrew formula. Manual setup instructions are in the README.

## v0.2.0, 2026-09-07

- The `◷` marker predicts Claude Code's resume-from-summary dialog: shown after the age in the list, and spelled out in the preview pane, when a session is over 100k tokens and has been idle for more than an hour.
- The preview pane shows the session's context token count.

## v0.1.1, 2026-09-07

- Fix: the default account (`~/.claude`) launched with `CLAUDE_CONFIG_DIR` set to its own path instead of left unset. Claude Code then looked for `.claude.json` inside that directory, found nothing, and opened the login wizard instead of resuming. Personal sessions now run with the variable explicitly removed from the environment; other accounts still get it set.

## v0.1.0, 2026-09-07

First release. Scans every `~/.claude*` account, indexes transcripts with an mtime-and-size cache, shows `/rename` names, live status and orphaned sessions (working directory gone), and opens the chosen session in a Ghostty tab, a tmux window, or in place.
