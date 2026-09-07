package claude

import "github.com/hamzafer/unpause/internal/config"

func rootFor(p string) config.Root { return config.Root{Label: "test", Path: p} }
