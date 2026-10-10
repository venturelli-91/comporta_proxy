// Package web embute o build do painel React no binário: um único artefato de deploy.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Painel devolve os arquivos de web/dist (vazio até rodar `npm run build`).
func Painel() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
