# Cowork sessions are a read-only Account

Claude Desktop's Cowork mode runs Claude Code inside a VM and keeps a separate config root for every Session under `~/Library/Application Support/Claude/local-agent-mode-sessions/`, next to a metadata file with the title Claude Desktop shows. The transcripts are standard Claude Code format, so unpause adds each root to the list under one Account, `cowork`, and takes the title from the metadata file.

They are listed and readable but never opened, renamed or autonamed. Their cwd is a path inside the VM (`/sessions/<name>`) or inside Claude Desktop's folder, and `claude --resume` on the host would append to a transcript Claude Desktop owns. The picker says to open them in Claude Desktop instead.

## Consequences

- The folder layout is Claude Desktop's internal detail. If it changes, the `cowork` Account silently goes empty; the detection lives in `internal/config` and has a fixture test.
- Being Cowork comes from where a root was found, not from its label.
