package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// coworkMeta is the part of Claude Desktop's local_<x>.json we use.
type coworkMeta struct {
	Title string `json:"title"`
}

// CoworkTitle returns the title Claude Desktop shows for the Cowork session whose config
// root is .../local_<x>/.claude. It reads the sibling .../local_<x>.json and returns ""
// when that file is missing, unreadable or has no title.
func CoworkTitle(root string) string {
	b, err := os.ReadFile(filepath.Dir(filepath.Clean(root)) + ".json")
	if err != nil {
		return ""
	}
	var m coworkMeta
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return strings.TrimSpace(m.Title)
}
