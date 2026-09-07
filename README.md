# unpause

**One list of every Claude Code session on your machine. Pick one, it opens where it left off.**

Claude Code's own `/resume` only shows sessions from the folder you're standing in. If you run it from many repos, or keep two accounts (`~/.claude` and `~/.claude-work`), finding yesterday's session means guessing the folder, guessing the account, and squinting at first prompts. unpause reads every account, every repo, shows the names you gave sessions with `/rename`, and resumes the one you pick with the right account in the right folder.

```
unpause  158 sessions · personal, work

› type to filter  ·  name, repo, account, branch, id

▶ ● ★ tracker-hertz                surgery-planning   work      now  │ tracker-hertz
  ● Openusage multiple accounts    playground         personal   2m  │ id       9e63e42f
    ★ interview-skill-brain        surgery-planning   work       3h  │ account  work
    Wishlist contents              track-one          personal   2d  │ repo     ~/developer/ai/deepinsight/surgery-planning
  ! Slack thread review            pow-1257           work       4d  │ branch   main
                                                                     │ active   now  ·  41 messages
                                                                     │ ● running  pid 9310  busy
                                                                     │
                                                                     │ you
                                                                     │ can you check why the hertz tracker drifts…
                                                                     │
                                                                     │ claude
                                                                     │ The drift comes from the 50 ms poll…

enter open  ^f fork  ^r rename  ↑↓ move  esc clear/quit   opens via ghostty-tab
```

## Install

```sh
brew install hamzafer/tap/unpause
# or
go install github.com/hamzafer/unpause/cmd/unpause@latest
```

## Use

```sh
unpause                 # the picker
unpause list            # plain text, newest first
unpause list --json     # for scripts
unpause open tracker    # resume by name or id prefix, no picker
unpause open 9e63 --print   # just print the shell command
unpause rename 9e63 "hertz tracker"
unpause doctor          # what it detected: accounts, opener, claude binary
```

Inside the picker:

| key | action |
|---|---|
| type | fuzzy filter on name, repo, account, branch, id |
| `enter` | open the session |
| `^f` | open as a fork (new session id, original untouched) |
| `^r` | rename (writes the same record Claude Code's `/rename` does) |
| `esc` | clear the filter, then quit |

Rows: `★` has a name, `●` running right now, `!` folder no longer exists.

## Where sessions open

unpause detects the terminal it's running in:

| you're in | a resumed session opens in |
|---|---|
| Ghostty | a new tab (via Ghostty's AppleScript API, Ghostty 1.3+) |
| tmux | a new window |
| anything else | in place of unpause |

Override with `--open tab|tmux|inplace` or in the config file. Tab and tmux openers leave the picker running so you can launch several sessions in a row.

Running sessions are never resumed twice: `enter` on a `●` row asks whether to fork it or open anyway.

## Accounts

Every `~/.claude` and `~/.claude-*` directory with a `projects/` folder is an account. `~/.claude` is labelled `personal`, `~/.claude-work` is `work`, and so on. `$CLAUDE_CONFIG_DIR` is honoured too. To relabel or add roots elsewhere:

```toml
# ~/.config/unpause/config.toml
opener = "auto"        # auto | tab | tmux | inplace
claude = "claude"      # binary to run

[[roots]]
label = "client"
path = "~/.claude-work"

[[roots]]
label = "lab"
path = "/Volumes/lab/.claude"
```

## How it works

unpause streams each transcript in `<account>/projects/*/*.jsonl` once, keeps a handful of fields (id, cwd, branch, timestamps, titles, the last few messages) and caches them in `~/.cache/unpause/index.json` keyed by mtime and size. First run over ~400 MB of transcripts takes about a second; after that it's milliseconds. Live status comes from Claude Code's pid files and is checked every run.

The transcript format is internal to Claude Code and may change. When it does, the parser in `internal/provider/claude/scan.go` is the only thing to fix. Decisions and their reasons are in [`docs/adr`](docs/adr); the vocabulary is in [`CONTEXT.md`](CONTEXT.md).

Claude Code today. Codex, OpenCode and friends are welcome as sibling providers.

## Development

```sh
go test ./...
go run ./cmd/unpause doctor
```

Releases: tag `vX.Y.Z`, then `goreleaser release --clean` with `GITHUB_TOKEN` and `HOMEBREW_TAP_GITHUB_TOKEN` set. The `release` workflow does the same on CI once the `HOMEBREW_TAP_GITHUB_TOKEN` secret exists.

MIT © Hamza Zafar
