package tui

import "os"

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}
