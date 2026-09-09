// Package web embeds the built frontend (web/dist). Run `make web` first; the
// tracked dist/.gitkeep keeps the embed pattern valid in a fresh checkout
// (the SPA then 404s until a real build exists).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend as a filesystem rooted at dist/.
func Dist() (fs.FS, error) { return fs.Sub(dist, "dist") }
