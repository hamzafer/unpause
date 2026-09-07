# Accounts are auto-detected from ~/.claude and ~/.claude-*

Two subscriptions on one machine means two `CLAUDE_CONFIG_DIR` directories, and the community convention is `~/.claude` plus `~/.claude-<something>` behind a shell alias. unpause treats every such directory that contains `projects/` as an Account, labels the default one `personal` and the others by their suffix, and also honours `$CLAUDE_CONFIG_DIR`. `~/.config/unpause/config.toml` can add roots elsewhere or relabel them, and `no_auto_detect` turns the scan off. Zero config for the common case was chosen over explicitness because a single-account user must never see a setup step.

## Consequences

- The default account must be launched with `CLAUDE_CONFIG_DIR` **unset**, never set to `~/.claude`. With the variable set, Claude Code looks for `.claude.json` inside that directory instead of at `~/.claude.json`, finds nothing, and starts the login wizard (this bit us in v0.1.0). `config.EnvFor` encodes the rule and every opener unsets the variable for the default account so an inherited value from a `claude-work` shell can't leak in.
