# The stats page is a Next.js static export embedded in the binary

`unpause mcp stats` answers "who read what" in a terminal; a page answers it at a glance and can update live. unpause ships as one Go binary through Homebrew, and the release pipeline has no Node. So the page is a Next.js static export committed under `internal/webui/dist` and served with `go:embed` by `unpause mcp stats --web`, which also serves a small JSON API built on the same `ReadLog` and `Summarize` as the CLI. A Go-templated page was rejected because the look (shadcn, charts, theming) is the point; a separately installed web app was rejected because it would be a second thing to install and keep in step.

The server binds `127.0.0.1` only, answers GET only, and refuses any request whose `Host` is not the local address it bound, so another site in the browser can't read the log through DNS rebinding. Fonts are bundled; the page makes no request off the machine.

## Consequences

- A change to the page means rebuilding the export and committing it. CI rebuilds it and fails when the committed files differ, so the binary never serves a stale page.
- The export has to build reproducibly: a fixed build id, a pinned Node and a lockfile.
- The binary grows by the size of the export, about 1.3 MB (React, Recharts and the Geist fonts).
