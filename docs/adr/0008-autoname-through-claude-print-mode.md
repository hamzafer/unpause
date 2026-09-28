# Autoname calls Haiku through `claude -p`

Sessions with no Name and no Claude title show the raw first prompt. `unpause autoname` fills that gap with a short name from Claude Haiku. Calling the Anthropic API directly would need an API key the user doesn't otherwise have; `claude -p --model haiku` uses the login each Account already holds, so the call runs under the session's own Account, with `CLAUDE_CONFIG_DIR` unset for the default one. It runs with `--no-session-persistence` so no naming Session shows up in the list, `--settings '{"disableAllHooks":true}'` plus an `UNPAUSE_AUTONAME` guard so the SessionEnd hook can't recurse, no tools and no MCP servers, from the temp dir so no project CLAUDE.md loads.

The name is written with the `/rename` record from ADR 0005, and only to Sessions that have neither a Name nor a Claude title, so nothing a person or Claude chose is replaced.

The hook can't do the work itself: Claude Code gives SessionEnd hooks about 1.5 seconds and a Haiku call takes several. So `unpause hook session-end` starts a detached `unpause autoname --session <id>` and returns. unpause does not install the hook into Claude Code's settings; that would be a second kind of write to another tool's files (ADR 0005). The README carries the snippet instead.

## Consequences

- Autoname depends on `claude -p` flags (`--model`, `--no-session-persistence`, `--settings`, `--tools`, `--strict-mcp-config`, `--system-prompt`). If one is renamed, naming calls fail and are reported; nothing is written.
- Each named Session costs one small Haiku call against the Account's plan.
- A Session that ends without the hook firing stays untitled until the next manual `unpause autoname`.
