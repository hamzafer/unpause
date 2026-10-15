# `unpause mcp stats --web`: the Access log as a page

Decided 2026-10-15. Look: `~/notes/dashboard-design.md` (Vercel Analytics, X console, ChatGPT usage). One scroll page, live (5 s polling while visible), a drill-in panel per Session. ADR 0014.

## Job

Answer one question at a glance: **who read my Sessions, and what did they read?** The reader is the owner, on their own machine, after connecting ChatGPT, Codex or Claude Desktop.

## Command and server (Go)

- `unpause mcp stats --web [--port 4777] [--no-open]`. Binds `127.0.0.1` only; if the port is busy, takes a free one. Prints the URL to stderr, opens the browser (`open` / `xdg-open`) unless `--no-open`, Ctrl+C stops.
- Rejects any request whose `Host` is not `127.0.0.1:<port>` or `localhost:<port>` (DNS rebinding). No CORS headers. GET only.
- Static files: the Next.js export, committed under `internal/webui/dist`, served with `go:embed`. goreleaser needs no Node. CI rebuilds the export and fails if `internal/webui/dist` differs.
- Data is read fresh on every API call from `ReadLog` + the index (titles), like the CLI.

### `GET /api/stats?range=24h|7d|30d|all`

```json
{
  "range": "7d",
  "generated_at": "2026-10-15T15:40:00Z",
  "empty": false,
  "kpis": {
    "calls": 24, "sessions_read": 6, "clients": 2, "errors": 0, "p50_ms": 73,
    "prev": { "calls": 3, "sessions_read": 1, "clients": 1, "errors": 0, "p50_ms": 90 }
  },
  "clients": [
    { "id": "local-agent-mode-unpause", "name": "Claude Cowork", "slot": 0, "calls": 20, "last_seen": "..." }
  ],
  "series": {
    "bucket": "hour",
    "points": [ { "t": "2026-10-15T15:00:00Z", "covered": true, "by_client": { "local-agent-mode-unpause": 3 } } ]
  },
  "most_read": [ { "id": "9ee240be-...", "title": "ChatGPT MCP for Claude chats", "account": "work", "project": "playground", "reads": 2, "last_read": "..." } ],
  "top_queries": [ { "query": "fxtwitter", "count": 1 } ],
  "tools": [ { "name": "get_session", "count": 2 } ],
  "recent": [ { "at": "...", "client": "local-agent-mode-unpause", "client_name": "Claude Cowork", "tool": "get_session", "input": { "id": "9ee240be-..." }, "session_ids": ["..."], "count": 169, "total": 553, "duration_ms": 73, "error": "" } ]
}
```

- `prev` is the same window just before (null for `all`). `p50_ms` over successful calls; null when none.
- Buckets: `24h` hourly (24), `7d` and `30d` daily, `all` daily from the first entry. `covered: false` for buckets before the first logged call (drawn as a gap, never zero). Zero calls inside coverage is a real zero.
- `slot`: a client's color slot, by first appearance in the whole log, so a client keeps its color across ranges. Slots 0 to 4; a 6th+ client folds into "Other" in the chart.
- `name`: friendly name: `local-agent-mode-*` Claude Cowork, `claude-ai` Claude Desktop, `openai-mcp` or names containing `chatgpt` ChatGPT, names starting `codex` Codex, else the raw id.
- `most_read`: top 10 by successful `get_session`. Titles from the local index; a Session no longer indexed shows its short id.
- `recent`: newest 50 calls in range.

### `GET /api/session?id=<id>`

`{ "id", "title", "account", "project", "calls": [ { "at", "client_name", "tool", "count" } ] }`, every logged call that returned this Session, newest first. 404 when unknown.

## Page (web)

Next.js App Router, `output: "export"`, `generateBuildId: "unpause"`, pnpm with a lockfile, shadcn/ui (new-york, zinc), shadcn charts (Recharts). Geist and Geist Mono self-hosted via the `geist` package. No request leaves the machine.

### Tokens

| Token | Dark | Light |
|---|---|---|
| background | `#000000` | `#ffffff` |
| card | `#0a0a0a` | `#fafafa` |
| border | `#1f1f1f` | `#e5e5e5` |
| text | `#ededed` | `#171717` |
| muted text | `#a1a1a1` | `#666666` |
| accent (UI) | `#3987e5` | `#2a78d6` |

Client series, validated with the dataviz validator against `#0a0a0a` and `#ffffff`: dark `#3987e5 #d95926 #199e70 #c98500 #d55181`, light `#2a78d6 #eb6834 #1baf7a #eda100 #e87ba4`. Light mode warns on contrast for slots 3 to 5, so the legend and table view are mandatory.

Type: Geist for UI, Geist Mono for numbers, ids, times and inputs. Scale 12 / 14 / 16 / 20 / 32. Sentence case everywhere, no all-caps labels.

### Layout (one scroll page, left aligned, max width 1200)

```
┌ unpause  MCP access ─────────────────────── [24h|7d|30d|All] ◐ ┐
│ Claude Cowork read 2 sessions in the last 24 hours.             │  ← the headline is a sentence
│ Calls 24 +700%   Sessions read 6   Clients 2   Errors 0   p50 73 ms │
├─────────────────────────────────────────────────────────────────┤
│ hero: calls over time, stacked area by client, live edge        │
├──────────────────────────────┬──────────────────────────────────┤
│ Most read sessions (bars)    │ Top queries (bars)               │
├──────────────────────────────┼──────────────────────────────────┤
│ Clients (bars, color dot)    │ Tools (bars)                     │
├──────────────────────────────┴──────────────────────────────────┤
│ Recent calls: time, client, tool, input, returned, error         │
└─────────────────────────────────────────────────────────────────┘
```

Under 768 px everything is one column; the KPI row wraps 2 by 3; the calls table scrolls inside its card, never the page.

### Principles

- **The headline is the answer.** A generated sentence names who read what in the range ("Claude Cowork read 2 sessions in the last 24 hours. ChatGPT read nothing."). It is the one bold element: 32 px Geist, tight tracking. Everything else is quiet.
- **One motion.** When polling brings a new call, the hero's live edge pulses once and the new row in Recent calls fades in from a brief accent tint. Nothing else animates; `prefers-reduced-motion` turns both off.
- Charts: thin marks, 2 px surface gaps in stacks, recessive grid, crosshair + tooltip, legend for 2+ clients, a "Table" toggle on the hero card. Ranked lists put a proportional bar behind each row (Vercel style), text in text colors.
- Delta chips: neutral gray chips with a sign and percent; no green/red (more calls is not good or bad). "new" when prev is 0.
- Drill-in: clicking a most-read row opens a shadcn `Sheet`: title, account, project, every call that returned it, and a copy button for `unpause open <short id>`. No chat content.
- Empty state (no log yet): the headline says "No MCP client has read your sessions yet." plus three lines on connecting Claude Desktop, Codex or ChatGPT (link to the README section). Sparse ranges render honestly.
- Errors: a call row with an error shows its class next to an icon; the KPI says "Errors 0" in muted text.
- Theme: dark by default, follows the system when the user hasn't picked; toggle in the header; choice kept in `localStorage`.
- Quality floor: visible focus rings, keyboard reachable sheet and toggles, 390 px without sideways scroll, `prefers-reduced-motion`.
