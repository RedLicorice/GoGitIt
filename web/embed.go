// Package web embeds the built Svelte single-page app so the compiled binary
// is self-contained — no web/dist directory needs to ship alongside it.
//
// `dist` is produced by `npm run build` (or `make build-frontend`). The
// committed dist/.gitkeep guarantees the directory exists so this package
// still compiles on a fresh checkout before the frontend is built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the built SPA as a filesystem rooted at the dist directory, so
// "index.html" sits at the top.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
