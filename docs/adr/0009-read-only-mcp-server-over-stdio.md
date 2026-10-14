# A read-only MCP server over stdio, reached through OpenAI's tunnel

The goal was to let ChatGPT read local Sessions. A separate Python server would have re-implemented Account detection, titles and live state, which unpause already has, so `unpause mcp` serves them itself with the official Go SDK. It speaks stdio only. ChatGPT needs an HTTPS endpoint, and OpenAI's Secure MCP Tunnel provides one without a listener on the Mac: `tunnel-client` runs `unpause mcp` as a child process and polls OpenAI over outbound HTTPS. An HTTP transport and a public tunnel (Cloudflare, ngrok) were rejected because they put an endpoint on the internet that serves raw transcripts.

The server has three tools (list, search, get) and no write path. Content is sent raw, tool output included, by the user's choice; `[mcp] exclude` globs on cwd hide Sessions from the server without hiding them from the picker. Search is a plain substring scan with no index, chosen over SQLite FTS and embeddings to keep the cache format and the dependency list unchanged.

## Consequences

- Everything a transcript holds, secrets in tool output included, can reach OpenAI while the tunnel runs. The tunnel is started by hand; unpause ships no service or launch agent for it.
- Search cost grows with transcript volume. If it nears the tunnel's 120 s request deadline, add an index then.
- `unpause mcp` must never print to stdout outside the protocol.
