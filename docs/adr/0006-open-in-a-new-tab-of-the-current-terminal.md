# Resume opens a new tab of the terminal you're in, not in place

The obvious design is `exec claude --resume` in the current process. The user wanted the picker to stay open so several sessions can be launched in a row, and did not use tmux. So the Opener is pluggable and auto-detected: Ghostty gets a real tab through its AppleScript dictionary (Ghostty 1.3+ ships `Ghostty.sdef` with `new tab` and `surface configuration`, including working directory, command and environment variables); inside tmux it is a new window; anything else falls back to in-place exec. `--open inplace|tmux|tab` overrides, and the config file sets a default.

## Consequences

- Tab openers run the session through `$SHELL -lic` so PATH, aliases and prompt hooks match a hand-opened tab.
- Warp, iTerm2, Kitty and WezTerm have no tab opener yet; they fall back to in-place. Each is a small file in `internal/opener`.
- Only Ghostty's dictionary was verified; keystroke-simulating hacks (System Events) were rejected as unreliable.

## Update: Warp support (v0.3.0)

Warp has no `--cwd`/`-e`-style CLI flag and its `warp://action/new_tab` URI takes a `path` but no startup command (confirmed against Warp's docs and an open, unresolved feature request for exactly that flag). The only documented way to open a new tab with both a working directory and a command is a Tab Config: a `.toml` file under `~/.warp/tab_configs/`, triggered by `open "warp://tab_config/<name>"`, matched case-insensitively against the file's stem. unpause writes one reusable file (`unpause-launch.toml`, overwritten on every launch) instead of accumulating one per session.
