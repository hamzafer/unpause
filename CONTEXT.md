# unpause

One list of every Claude Code session on a machine, across every repo and every account. Pick one and it opens where it left off.

## Language

**Session**:
One resumable conversation with a coding agent. Identified by the agent's own session id.
_Avoid_: chat, conversation, thread, agent

**Transcript**:
The on-disk file the agent writes for a Session. unpause reads it, never rewrites it.
_Avoid_: log, history file

**Account**:
One Claude Code configuration directory (what `CLAUDE_CONFIG_DIR` points at), usually tied to one subscription. Has a short label such as `personal` or `work`.
_Avoid_: profile, root (in user-facing text), subscription

**Root**:
The filesystem path of an Account. Internal term only; users see the Account label.

**Provider**:
The coding agent a Session belongs to. Claude Code today; the slot exists for others.
_Avoid_: tool, backend, source

**Name**:
The title a person gave a Session with `/rename` (or `unpause rename`). Optional.
_Avoid_: custom title, label

**Title**:
What a Session is called in the list: its Name if it has one, else the agent's auto-generated summary, else the first prompt.

**Live**:
A Session whose agent process is running right now.
_Avoid_: active, running, open

**Orphan**:
A Session whose working directory no longer exists on disk. Still listed, flagged, resumable elsewhere.
_Avoid_: stale, broken, dead

**Opener**:
The strategy for where a resumed Session appears: a new tab of the current terminal, a tmux window, or in place of the unpause process.
_Avoid_: launcher, target, spawner

**Fork**:
Resuming a Session under a new session id so the original is left untouched.
_Avoid_: copy, branch, clone
