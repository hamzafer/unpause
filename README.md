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
  ! Slack thread review            pow-1257           work       4d◷ │ branch   main
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
unpause                      # the picker
unpause --all                # picker, including empty sessions and subagent transcripts
unpause --open tmux          # override the opener for this run
unpause list                 # plain text, newest first
unpause list --json          # for scripts
unpause list --all           # include empty sessions and subagent transcripts
unpause open tracker         # resume by name or id prefix, no picker
unpause open 9e63 --print    # just print the shell command, don't run it
unpause open 9e63 --fork     # resume as a fork (new session id, original untouched)
unpause rename 9e63 "hertz tracker"
unpause doctor                 # what it detected: accounts, opener, claude binary
unpause --version            # print the binary version
```

By default the list hides empty sessions (nobody ever typed a prompt) and subagent transcripts. `--all`, on the top-level command or on `list`, shows them too.

Inside the picker:

| key | action |
|---|---|
| type | fuzzy filter on name, repo, account, branch, id |
| `enter` | open the session (asks first if it's running, see below) |
| `^f` | open as a fork (new session id, original untouched) |
| `^r` | rename (writes the same record Claude Code's `/rename` does) |
| `↑`/`↓`, `^p`/`^n`, `^k`/`^j` | move the cursor one row |
| `pgup`/`pgdown` | move a page at a time |
| `home`/`end` | jump to the first/last row |
| `esc` | clear the filter, then quit |

`enter` on a row that's running right now (`●`) doesn't resume it; it opens a small prompt instead:

| key | action |
|---|---|
| `f` or `enter` | fork a copy |
| `o` | open anyway |
| `esc` | cancel |

`^r` turns the filter box into a rename prompt: `enter` saves the name, `esc` cancels without writing anything.

Rows: `★` has a name, `●` running right now, `!` folder no longer exists (an orphan; opening it resumes the transcript in your home directory instead), `◷` after the age means Claude Code will ask whether to resume from a summary (the session is over 100k tokens and has been idle for more than an hour). That last one is a prediction from the transcript's last usage record: the thresholds are approximate, and unpause can't tell if you've already picked "Don't ask again".

## Where sessions open

unpause detects the terminal it's running in:

| you're in | a resumed session opens in |
|---|---|
| Ghostty | a new tab (via Ghostty's AppleScript API, Ghostty 1.3+) |
| Warp | a new tab (via a reusable Warp Tab Config) |
| tmux | a new window |
| anything else | in place of unpause |

Override with `--open tab|tmux|inplace|ghostty|warp` or in the config file. Tab and tmux openers leave the picker running so you can launch several sessions in a row.

Running sessions are never resumed twice; see the fork/open-anyway prompt above.

## Accounts

Every `~/.claude` and `~/.claude-*` directory with a `projects/` folder is an account. `~/.claude` is labelled `personal`, `~/.claude-work` is `work`, and so on. `$CLAUDE_CONFIG_DIR` is honoured too. The `personal` account (`~/.claude`) always launches with `CLAUDE_CONFIG_DIR` unset, never set to its own path: Claude Code treats an explicitly-set value as a request to look for `.claude.json` inside that directory, finds nothing, and opens the login wizard instead of resuming. Other accounts launch with the variable set to their path.

To relabel an account or add one at a path unpause wouldn't otherwise find:

```toml
# ~/.config/unpause/config.toml
opener = "auto"          # auto | tab | tmux | inplace
claude = "claude"        # binary to run
no_auto_detect = false   # true disables the ~/.claude* scan; only [[roots]] below are used

[[roots]]
label = "client"
path = "~/.claude-work"

[[roots]]
label = "lab"
path = "/Volumes/lab/.claude"
```

`opener` and `claude` are optional and default to `auto` and `claude`. `[[roots]]` entries add accounts or override the label of an auto-detected one at the same path; `~` is expanded.

## How it works

unpause streams each transcript in `<account>/projects/*/*.jsonl` once, keeps a handful of fields (id, cwd, branch, timestamps, titles, the last few messages) and caches them in `~/.cache/unpause/index.json`, keyed by each transcript's path, mtime and size. A transcript is re-parsed only when its mtime or size changes, or after `unpause rename` writes to it. First run over ~400 MB of transcripts takes about a second; after that it's milliseconds. It's safe to delete the cache file at any time; unpause rebuilds it from scratch on the next run. Live status comes from Claude Code's pid files and is checked every run, never cached.

The transcript format is internal to Claude Code and may change. When it does, the parser in `internal/provider/claude/scan.go` is the only thing to fix. Decisions and their reasons are in [`docs/adr`](docs/adr); the vocabulary is in [`CONTEXT.md`](CONTEXT.md); how to work on the code is in [`CONTRIBUTING.md`](CONTRIBUTING.md).

Claude Code today. Codex, OpenCode and friends are welcome as sibling providers.

## Shell completions

Installed automatically by the Homebrew formula. To set them up manually:

```sh
unpause completion zsh  > "${fpath[1]}/_unpause"     # zsh
unpause completion bash > /etc/bash_completion.d/unpause
unpause completion fish > ~/.config/fish/completions/unpause.fish
```

## Development

```sh
go vet ./...
go test ./...
go run ./cmd/unpause doctor
```

If a change alters what the parser reads out of a transcript, bump `cacheVersion` in `internal/index/index.go` so cache entries built by the old code get re-parsed instead of reused; see [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full checklist.

Releases: tag `vX.Y.Z`, then `goreleaser release --clean` with `GITHUB_TOKEN` and `HOMEBREW_TAP_GITHUB_TOKEN` set. The `release` workflow does the same on CI once the `HOMEBREW_TAP_GITHUB_TOKEN` secret exists. See [`CHANGELOG.md`](CHANGELOG.md) for release history.

MIT © Hamza Zafar
