package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// handler serves the SPA and falls back to index.html for client-side routes. A
// missing path with an extension or under /assets/ is a real 404, so a missing
// hashed asset never answers with HTML. /docs is served from the VitePress
// build in dist/docs and never falls back to the SPA.
func handler(sub fs.FS, hosted bool) http.Handler {
	files := http.FileServerFS(sub)
	var index []byte
	if hosted {
		var err error
		index, err = fs.ReadFile(sub, "index.html")
		if err != nil {
			panic(err)
		}
		index = bytes.Replace(index, []byte("</head>"),
			[]byte(`<meta name="tstamps-hosted" content="1"></head>`), 1)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}

		fallback := false
		if file, under := resolveDocs(name, sub); under {
			if file == "" {
				http.NotFound(w, r)
				return
			}
			name = file
		} else if info, err := fs.Stat(sub, name); err == nil && info.IsDir() {
			name = path.Join(name, "index.html")
			if _, err := fs.Stat(sub, name); err != nil {
				http.NotFound(w, r)
				return
			}
		} else if _, err := fs.Stat(sub, name); err != nil {
			if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
			fallback = true
		}
		r = r.Clone(r.Context())
		if name == "index.html" || strings.HasSuffix(name, "/index.html") {
			// Only the landing page is meant to be indexed; every other client
			// route is the same shell, so keep crawlers out of it.
			if fallback {
				w.Header().Set("X-Robots-Tag", "noindex, nofollow")
			}
			// FileServer redirects an explicit index.html path to its directory,
			// which would loop with clean URLs. Serve the directory instead.
			w.Header().Set("Cache-Control", "no-store")
			if hosted && name == "index.html" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(index)
				return
			}
			r.URL.Path = "/" + strings.TrimSuffix(name, "index.html")
		} else {
			if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "docs/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = "/" + name
		}
		files.ServeHTTP(w, r)
	})
}
