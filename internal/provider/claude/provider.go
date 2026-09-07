package claude

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzafer/unpause/internal/config"
)

// Transcript is a candidate session file with the metadata needed for cache lookups.
type Transcript struct {
	Root    config.Root
	Path    string
	ModTime int64
	Size    int64
}

// ListTranscripts finds every top-level session transcript under root/projects.
// Subagent transcripts live in a subdirectory named after the session and are skipped.
func ListTranscripts(root config.Root) ([]Transcript, error) {
	projects := filepath.Join(root.Path, "projects")
	dirs, err := os.ReadDir(projects)
	if err != nil {
		return nil, err
	}
	var out []Transcript
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(projects, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			out = append(out, Transcript{
				Root:    root,
				Path:    filepath.Join(projects, d.Name(), f.Name()),
				ModTime: info.ModTime().UnixNano(),
				Size:    info.Size(),
			})
		}
	}
	return out, nil
}
