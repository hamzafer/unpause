// Package webui holds the stats page: the Next.js static export under dist, built from
// web/ and committed, so the binary serves it without Node (ADR 0014).
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS is the export with dist as its root: index.html, _next/..., and so on.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path; Sub can't fail on it
	}
	return sub
}
