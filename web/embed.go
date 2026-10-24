//go:build web

// Package web serves the built single-page application from the binary.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves dist and falls back to index.html for client-side routes. A
// missing path with an extension or under /assets/ is a real 404, so a missing
// hashed asset never answers with HTML.
func Handler() http.Handler { return handler(false) }

// HostedHandler marks the shell as running in hosted mode, so the SPA asks for a
// workspace before showing a login form.
func HostedHandler() http.Handler { return handler(true) }

func handler(hosted bool) http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // //go:embed all:dist guarantees the tree exists
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}
	if hosted {
		index = bytes.Replace(index, []byte("</head>"),
			[]byte(`<meta name="tstamps-hosted" content="1"></head>`), 1)
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if _, err := fs.Stat(sub, name); err != nil {
			if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		r = r.Clone(r.Context())
		if name == "index.html" {
			if hosted {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(index)
				return
			}
			// FileServer redirects a /index.html path to its directory, so serve
			// the directory itself and keep client routes on their own URL.
			w.Header().Set("Cache-Control", "no-store")
			r.URL.Path = "/"
		} else {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = "/" + name
		}
		files.ServeHTTP(w, r)
	})
}
