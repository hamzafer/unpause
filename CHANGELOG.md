# Changelog

## Unreleased

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
