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
The title a Session was given with `/rename`, `unpause rename`, or `unpause autoname` (which only names Sessions that have no Name and no auto-generated summary). Optional.
_Avoid_: custom title, label

**Recap**:
Three short lines (goal, done, next) a small model writes about a Session, shown in the picker's preview. Opt-in, kept in unpause's cache, never in the Transcript.
_Avoid_: summary (that is Claude Code's auto-generated title), digest, AI summary

**Title**:
What a Session is called in the list: its Name if it has one, else the title Claude Desktop shows (Cowork Sessions only), else the agent's auto-generated summary, else the first prompt.

**Deep search**:
Finding a Session by what was said in it: its talk (your prompts and Claude's replies, not tool calls, tool output or thinking) plus its Title, unless that is just the first prompt. Every term must appear; ranked by matches. Shared by the picker, `unpause search` and the MCP server.
_Avoid_: full-text search, grep

**Live**:
A Session whose agent process is running right now.
_Avoid_: active, running, open

**Orphan**:
A Session whose working directory no longer exists on disk and is not a Retired worktree. Still listed, flagged, resumable elsewhere.
_Avoid_: stale, broken, dead

**Retired worktree**:
A Session whose Claude Code worktree (`<repo>/.claude/worktrees/<name>`) was removed while its repo still exists. Listed calmly, not flagged; resumes in the repo.
_Avoid_: finished, merged, closed

**Opener**:
The strategy for where a resumed Session appears: a new tab of the current terminal, a tmux window, or in place of the unpause process.
_Avoid_: launcher, target, spawner

**Fork**:
Resuming a Session under a new session id so the original is left untouched.
_Avoid_: copy, branch, clone

**Cowork Session**:
A Session that Claude Desktop's Cowork mode ran inside its VM. Listed under the `cowork` Account and readable, but never opened, renamed or autonamed by unpause.
_Avoid_: desktop session, agent-mode session

**MCP server**:
`unpause mcp`: the read-only stdio server that lets another assistant (Claude Desktop, the ChatGPT Mac app, Codex, or ChatGPT on the web through OpenAI's tunnel) list, search and read Sessions.
_Avoid_: plugin, connector, API

**Excluded**:
A Session that worked in a folder matching an `[mcp] exclude` glob: any cwd it ever had, or, for a Cowork Session, a folder the user gave it in Claude Desktop. While any glob is set, a Session with no folder at all is Excluded too. Invisible to the MCP server, still shown in the picker.
_Avoid_: hidden, blocked, denylisted

**Access log**:
The MCP server's record of every tool call: when, which client, which tool, which inputs, which Sessions came back. Never holds message text.
_Avoid_: audit trail, history, analytics
