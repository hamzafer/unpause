package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// CoworkMeta is the part of Claude Desktop's local_<x>.json unpause uses.
type CoworkMeta struct {
	// Title is what Claude Desktop shows for the session.
	Title string
	// Folders are the host folders the user gave the session (userSelectedFolders).
	Folders []string
	// CWD is the session's working directory inside the VM.
	CWD string
}

// ReadCowork reads the metadata of the Cowork session whose config root is
// .../local_<x>/.claude, from the sibling .../local_<x>.json. A missing or
// unreadable file gives the zero CoworkMeta.
func ReadCowork(root string) CoworkMeta {
	b, err := os.ReadFile(filepath.Dir(filepath.Clean(root)) + ".json")
	if err != nil {
		return CoworkMeta{}
	}
	var raw struct {
		Title   string   `json:"title"`
		Folders []string `json:"userSelectedFolders"`
		CWD     string   `json:"cwd"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return CoworkMeta{}
	}
	m := CoworkMeta{Title: strings.TrimSpace(raw.Title), CWD: strings.TrimSpace(raw.CWD)}
	for _, f := range raw.Folders {
		if f = strings.TrimSpace(f); f != "" {
			m.Folders = append(m.Folders, f)
		}
	}
	return m
}
