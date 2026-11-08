// Package frontend embeds the built desktop frontend. Build it with
// `npm ci && npm run build` in this folder before `go build`; without a build
// the window is empty.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend files.
func Dist() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded, so this cannot fail
	}
	return files
}
