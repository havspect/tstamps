//go:build web

// Package web serves the built single-page application from the binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var dist embed.FS

// Handler serves dist, including the VitePress build under dist/docs.
func Handler() http.Handler { return handler(openDist(), false) }

// HostedHandler marks the shell as running in hosted mode, so the SPA asks for a
// workspace before showing a login form.
func HostedHandler() http.Handler { return handler(openDist(), true) }

func openDist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // //go:embed all:dist guarantees the tree exists
	}
	return sub
}
