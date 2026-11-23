// Package web serves the embedded UI bundle built by `just build ui`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built UI file tree.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time; absence is a build bug
	}
	return sub
}
