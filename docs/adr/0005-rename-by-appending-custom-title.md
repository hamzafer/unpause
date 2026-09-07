# Rename writes into Claude Code's transcript

The user's core complaint was that `/rename` names never surfaced. Keeping names in unpause's own store would fix the list but not Claude's own picker, and the name would not follow `claude --resume`. Instead, `unpause rename` appends the exact record Claude Code writes for `/rename` (`{"type":"custom-title","customTitle":…,"sessionId":…}`) to the transcript. This is the only write unpause ever makes to another tool's files, and it is strictly append-only: no existing line is read back, rewritten or removed.

## Consequences

- If Claude Code changes the record shape, renames from unpause stop showing in Claude's picker until the writer is updated. The reader in the same package must keep accepting both shapes.
- Never extend this into editing or deleting transcript content.
