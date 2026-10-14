# Deep search

Decided in a grill session on 2026-10-14. Two vertical slices; slice 2 builds on slice 1.

## Goal

Find a Session by what was said in it, not just by its Title. The picker gets a Deep search mode, the CLI gets `unpause search`, and the MCP server's `search_sessions` switches to the same rules so one query gives the same answer everywhere.

## Rules (shared by picker, CLI and MCP)

- **Scope: talk only.** Text blocks of user and assistant records (what you typed, what Claude replied), plus the Session's Title when it is a Name, Desktop title or AI title (a Title made from the first prompt would count that prompt twice). Tool calls, tool results and thinking are not searched.
- **Query.** Split into terms on whitespace; `"double quotes"` make one phrase term (an unclosed quote runs to the end). Case-insensitive substring per term. A Session matches when **every** term appears somewhere in it, not necessarily in the same message. An empty query (no terms) matches nothing.
- **Hits** = total occurrences of all terms. **Snippets**: up to 3 per Session, about 90 characters each side of a match, in file order (Title matches give no snippet).
- **Ranking**: hits descending, ties by last activity, newest first.
- **No index.** Transcripts are scanned in parallel on each search (ADR 0011). A transcript that can't be read just doesn't match.

## Slice 1: matcher, shared search, MCP and CLI

`internal/provider/claude`:

- `NewMatcher(q)` parses terms as above. The per-line pre-filter checks the lowercased raw line for any term's JSON-escaped form before decoding.
- `SearchFile` returns hits and snippets for talk text only, plus which terms were seen, so the caller can apply the every-term rule.
- New `Search(ctx, sessions, query, progress)`: runs `SearchFile` over the given Sessions with `GOMAXPROCS` workers, adds Title matches, keeps Sessions where every term matched, and returns them ranked. `progress(done, total)` is optional and is called as files finish. Honors ctx cancellation (returns ctx.Err()).

`internal/mcpserver`: `search_sessions` calls `claude.Search` instead of its own loop. The tool description and the `query` schema text describe the new rules. Paging is unchanged. Results are now ranked instead of newest first.

`cmd/unpause`: `unpause search <query...>` (args joined with spaces, so `unpause search tunnel keychain` works without quotes):

```
  12×  3f2a9c1b  work      unpause                  2d   unpause mcp tunnel
       …store the runtime key in Keychain for the tunnel…
```

One line per match like `unpause list` (live dot, hit count, short id, account, repo, age, title), then the first snippet indented on the next line. `--all` as in `list`. `--json` prints `[{session, hits, snippets}]`. `--limit N` (default 20, 0 = all). No match: prints nothing and exits 0.

Tests: matcher fixtures for talk-only (a term only in a tool result or thinking does not match), every-term across messages, phrase quotes, ranking, cancellation; MCP tests updated for talk-only and ranking; a CLI test.

## Slice 2: Deep search in the picker

- `^s` switches the filter box into Deep search: prompt `deep › `, placeholder `search inside transcripts  ·  "quotes" for a phrase`. The current filter text carries over as the first query. `^s` again or `esc` goes back to the normal filter (the query clears); `esc` in the normal filter keeps today's behavior (clear, then quit).
- Typing starts a search 300 ms after the last keystroke. A new keystroke cancels the running search (context cancel) and restarts the timer. Stale results (older search id) are dropped.
- While searching, the header shows `searching 430/1175`; after, `deep · N match`. The list keeps the previous results until the new ones arrive.
- The list shows only matching Sessions, ranked, with a dim `12×` hit count before the title. Cursor goes to the top on new results.
- The preview pane keeps its metadata block, then shows the snippets (instead of the last messages) with every term highlighted in the accent color.
- `enter`, `^f`, `^r` and the live-confirm prompt work on the selected result as usual. After a rename the results stay.
- The footer shows `^s back` in Deep search, `^s deep search` in the normal filter.

The scan runs on `tui.Options.Search` (a `func(ctx, query, progress) ([]Result, error)` injected by `main.go`), so the TUI stays testable without transcripts.

Tests: model tests with a fake Search: debounce produces one search, a stale result is ignored, `esc` returns to filter mode, ranked list and hit counts render, snippets highlight.

## Docs

ADR 0011. CONTEXT.md: **Deep search** term. README: picker keys, `unpause search`, MCP `search_sessions` text. CHANGELOG `## Unreleased`. WISHLIST: drop the "Search inside" line.

## Amendments after review (PR #9 and follow-up)

- The pre-filter always decodes lines containing `\u` or `\/` escapes, since a term can be spelled differently there.
- `isMeta` user records (injected reminders, skill text) are not talk and are skipped.
- Snippets turn control characters into spaces.
- `unpause search "two words"`: one argument with spaces and no quotes of its own was quoted in the shell, so it is a phrase.
- `tui.Options.Search` takes the Sessions to search: the picker passes copies taken on the Bubble Tea goroutine and maps results back to its own Sessions, so a rename never races a search.
- The rename and live-confirm prompts act on the Session they were opened for, even if new results arrive while they are open.
- A failed search shows `search failed` in the list until the next query.
