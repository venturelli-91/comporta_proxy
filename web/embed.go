// Package web embeds the React dashboard build in the binary: a single deploy artifact.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Painel returns the files in web/dist (empty until `npm run build` runs).
func Painel() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
