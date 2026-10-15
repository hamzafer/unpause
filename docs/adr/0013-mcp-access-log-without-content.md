# The MCP server keeps an access log, without chat content

Once `unpause mcp` serves raw transcripts to another assistant, the user needs to see who read what: which client called which tool, with which inputs, and how much came back. Claude Desktop logs MCP traffic on its own, but ChatGPT through the tunnel leaves nothing on the Mac. So the server appends one JSON line per tool call to `access.jsonl` next to the index cache, and `unpause mcp stats` summarizes it.

A line records the time, the client name and version from the MCP handshake, the tool, its inputs (query, account, project, id, paging), the Session ids returned, the result count, the duration and any error. It never records message text, titles or snippets: a log that quoted them would be a second, unguarded copy of the transcripts. Inputs are what the client sent, so a search query does land in the log.

## Consequences

- The log grows with use. It rotates to `access.jsonl.1` at 10 MB; one old file is kept.
- A failed log write never fails a tool call; it is reported on stderr.
- When OAuth arrives, the caller's identity belongs in the same line.
