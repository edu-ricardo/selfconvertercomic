// Package web embute o build estático do frontend (SvelteKit) no binário.
package web

import (
	"embed"
	"io/fs"
)

// build/ é gerado por `npm run build`; no git fica só o .gitkeep.
//
//go:embed all:build
var build embed.FS

// Files devolve o conteúdo de build/ como raiz.
func Files() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic(err) // impossível: "build" está embutido acima
	}
	return sub
}
