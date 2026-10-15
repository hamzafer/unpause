# The MCP server keeps an access log, without chat content

Once `unpause mcp` serves raw transcripts to another assistant, the user needs to see who read what: which client called which tool, with which inputs, and how much came back. Claude Desktop logs MCP traffic on its own, but ChatGPT through the tunnel leaves nothing on the Mac. So the server appends one JSON line per tool call to `access.jsonl` next to the index cache, and `unpause mcp stats` summarizes it.

A line records the time, the client name and version from the MCP handshake, the tool, its inputs (query, account, project, id, paging), the Session ids returned, the result count, the duration and any error. It never records message text, titles or snippets: a log that quoted them would be a second, unguarded copy of the transcripts. Inputs are what the client sent, so a search query does land in the log.

## Consequences

- The log grows with use. It rotates to `access.jsonl.1` at 10 MB; one old file is kept.
- A failed log write never fails a tool call; it is reported on stderr.
- Every string the client sends is cut to 500 bytes, so a line stays small. `unpause mcp stats` reads past a line that is too long or broken instead of failing.
- Errors are logged as one of four classes (`not found`, `invalid input`, `canceled`, `failed`), never their text, which can name paths. Calls the SDK rejects before a handler runs are logged as `invalid input`.
- Writes take an advisory lock (`access.jsonl.lock`) around rotation and append, so two servers can't both rotate and lose a generation. Where flock is missing, only the in-process lock holds.
- The log is never written through a symlink or to anything but a regular file; a refusal is a failed write, reported on stderr.
- When OAuth arrives, the caller's identity belongs in the same line.
