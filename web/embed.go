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

// HostedHandler marks the shell as a server shell: the root is the homepage and
// workspace prompt, and every workspace lives under /w/{slug}. The marker lets
// the SPA show the prompt instead of a login form at the root. Options carries
// the demo accounts and the legal-page marker.
func HostedHandler(opts ...Options) http.Handler { return handler(openDist(), true, opts...) }

func openDist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // //go:embed all:dist guarantees the tree exists
	}
	return sub
}
