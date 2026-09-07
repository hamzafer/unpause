# Contributing

## Running tests

```sh
go vet ./...
go test ./...
```

Fixture-based tests live next to the code they test, e.g. `internal/provider/claude/scan_test.go` reads `internal/provider/claude/testdata/*.jsonl`. `go run ./cmd/unpause doctor` is the fastest way to sanity-check against your own real transcripts.

## Where things live

- `internal/session`: the provider-agnostic `Session` model and small formatting helpers (`Age`, `Clip`, `Tokens`). Every provider parses into this struct; the TUI and `list`/`open`/`rename` only ever see it.
- `internal/provider/claude`: the Claude Code parser (`scan.go`), the rename writer (`rename.go`), live-session detection (`live.go`), and transcript discovery (`provider.go`).
- `internal/index`: builds the full session list across accounts, with the mtime-and-size cache.
- `internal/config`: reads `~/.config/unpause/config.toml` and auto-detects `~/.claude*` accounts.
- `internal/opener`: where a resumed session appears (new tab, tmux window, in place).
- `internal/tui`: the interactive picker (Bubble Tea).
- `cmd/unpause`: the Cobra CLI wiring.

Decisions and their reasoning are in [`docs/adr`](docs/adr); the shared vocabulary (account, session, provider, opener, fork, orphan, ...) is in [`CONTEXT.md`](CONTEXT.md). Read both before changing behavior, they explain the "why", and new code should use the same words.

## Adding a provider

unpause parses Claude Code today; the `Provider` field on `session.Session` and the package split under `internal/provider/` exist so a sibling agent can be added without touching the list, cache or opener (see [ADR 0002](docs/adr/0002-claude-code-first-with-provider-slot.md)).

To add one:

1. Create `internal/provider/<name>/` with a scanner that turns that agent's on-disk transcript into a `*session.Session` (see `internal/provider/claude/scan.go` for the shape: id, cwd, branch, timestamps, titles, a handful of preview messages).
2. Give it a `ListTranscripts`-equivalent that returns candidate files with path, mtime and size, so `internal/index` can cache them the same way it caches Claude Code's.
3. Set `Session.Provider` to the new provider's name.
4. Wire discovery into `internal/index.Build` (or wherever roots are resolved for that provider) and, if the provider needs its own account/root concept, extend `internal/config`.
5. If the provider supports resuming a session id, either reuse `internal/opener.Launch` (it already carries `Claude` as "the binary to run") or extend it; don't invent a parallel launch path.

## Adding an opener

Openers decide where a resumed session appears. Add a new file in `internal/opener/` implementing:

```go
type Opener interface {
    Name() string
    Open(l Launch) error
}
```

If your opener needs to run after the TUI has released the terminal (because it replaces the current process, like `InPlace`, or has to exec into something else to attach), also implement `Detacher`:

```go
type Detacher interface {
    Detach() bool
}
```

`opener.NeedsDetach` checks for this; the TUI hands the `Launch` back to `main.go` instead of calling `Open` itself when it's true. Wire the new opener into `terminalTab()` or `Detect()` in `internal/opener/opener.go` so `--open auto` can find it, and add a case to `Pick` if it should also be selectable by name.

Warp, iTerm2, Kitty and WezTerm don't have tab openers yet and fall back to in-place; each is meant to be one small file like `ghostty.go`.

## Changing the transcript parser

Claude Code's transcript format is internal and can change between releases (see [ADR 0004](docs/adr/0004-parse-transcripts-directly-with-mtime-cache.md)). If you touch what `internal/provider/claude/scan.go` reads out of a transcript, a field added, renamed, or reinterpreted:

1. Update `internal/provider/claude/testdata/*.jsonl` to cover the new shape, and the tests in `scan_test.go` that assert against it.
2. Bump `cacheVersion` in `internal/index/index.go`. The cache is keyed by transcript path, mtime and size, not by parser version, so without this bump, old cache entries built by the previous parser get silently reused forever instead of being re-parsed.
3. Run `go test ./...` and, if you have real transcripts handy, `go run ./cmd/unpause doctor` and `go run ./cmd/unpause list` to eyeball the result.
