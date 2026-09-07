# Go with Bubble Tea for the TUI

Considered Go, TypeScript on Bun, Rust with ratatui, and bash with fzf. Go gives a single static binary, `brew install` and `go install` for free, and is the idiomatic choice for terminal tools people already trust (gh, lazygit, sesh). Bun binaries are large and Ink is a weaker TUI layer; Rust is slower to iterate for a solo project; bash plus fzf makes fzf a hard dependency and cannot grow into preview, rename or live status. Swapping language later would be a rewrite, so this is fixed.
