// Package web embeds the built frontend (web/dist). Run `make web` first; the
// placeholder index.html keeps the embed valid in a fresh checkout.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend as a filesystem rooted at dist/.
func Dist() (fs.FS, error) { return fs.Sub(dist, "dist") }
