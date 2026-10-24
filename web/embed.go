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
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // //go:embed all:dist guarantees the tree exists
	}
	return handler(sub)
}
