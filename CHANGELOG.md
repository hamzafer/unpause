# Changelog

## v0.2.0, 2026-09-07

- The `◷` marker predicts Claude Code's resume-from-summary dialog: shown after the age in the list, and spelled out in the preview pane, when a session is over 100k tokens and has been idle for more than an hour.
- The preview pane shows the session's context token count.

## v0.1.1, 2026-09-07

- Fix: the default account (`~/.claude`) launched with `CLAUDE_CONFIG_DIR` set to its own path instead of left unset. Claude Code then looked for `.claude.json` inside that directory, found nothing, and opened the login wizard instead of resuming. Personal sessions now run with the variable explicitly removed from the environment; other accounts still get it set.

## v0.1.0, 2026-09-07

First release. Scans every `~/.claude*` account, indexes transcripts with an mtime-and-size cache, shows `/rename` names, live status and orphaned sessions (working directory gone), and opens the chosen session in a Ghostty tab, a tmux window, or in place.
