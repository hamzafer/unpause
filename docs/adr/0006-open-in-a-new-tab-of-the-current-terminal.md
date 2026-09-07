# Resume opens a new tab of the terminal you're in, not in place

The obvious design is `exec claude --resume` in the current process. The user wanted the picker to stay open so several sessions can be launched in a row, and did not use tmux. So the Opener is pluggable and auto-detected: Ghostty gets a real tab through its AppleScript dictionary (Ghostty 1.3+ ships `Ghostty.sdef` with `new tab` and `surface configuration`, including working directory, command and environment variables); inside tmux it is a new window; anything else falls back to in-place exec. `--open inplace|tmux|tab` overrides, and the config file sets a default.

## Consequences

- Tab openers run the session through `$SHELL -lic` so PATH, aliases and prompt hooks match a hand-opened tab.
- Warp, iTerm2, Kitty and WezTerm have no tab opener yet; they fall back to in-place. Each is a small file in `internal/opener`.
- Only Ghostty's dictionary was verified; keystroke-simulating hacks (System Events) were rejected as unreliable.
