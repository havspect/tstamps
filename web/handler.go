package web

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Options modifies the served shell. Zero value is the production shell.
type Options struct {
	// DemoAccounts is a JSON array stamped into the shell so the login page can
	// show the public demo sign-ins. Empty on any other server.
	DemoAccounts string

	// Legal stamps the marker that makes the SPA link to /imprint and /privacy.
	Legal bool
}

// handler serves the SPA and falls back to index.html for client-side routes. A
// missing path with an extension or under /assets/ is a real 404, so a missing
// hashed asset never answers with HTML. /docs is served from the VitePress
// build in dist/docs and never falls back to the SPA.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func handler(sub fs.FS, hosted bool, opts ...Options) http.Handler {
	var options Options
	if len(opts) > 0 {
		options = opts[0]
	}
	files := http.FileServerFS(sub)
	indexTemplate, _ := fs.ReadFile(sub, "index.html")
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
			if name == "index.html" && indexTemplate != nil {
				orig := requestOrigin(r)
				out := bytes.Replace(indexTemplate, []byte(`<link rel="canonical" href="/"`), []byte(`<link rel="canonical" href="`+orig+`/"`), 1)
				out = bytes.Replace(out, []byte(`<meta property="og:url" content="/"`), []byte(`<meta property="og:url" content="`+orig+`/"`), 1)
				out = bytes.Replace(out, []byte(`content="/screenshots/timeline.webp"`), []byte(`content="`+orig+`/screenshots/timeline.webp"`), -1)
				if hosted && !bytes.Contains(out, []byte(`name="tstamps-hosted"`)) {
					out = bytes.Replace(out, []byte("</head>"),
						[]byte(`<meta name="tstamps-hosted" content="1"></head>`), 1)
				}
				if options.DemoAccounts != "" {
					out = bytes.Replace(out, []byte("</head>"),
						[]byte(`<meta name="tstamps-demo" content="`+html.EscapeString(options.DemoAccounts)+`"></head>`), 1)
				}
				if options.Legal && !bytes.Contains(out, []byte(`name="tstamps-legal"`)) {
					out = bytes.Replace(out, []byte("</head>"),
						[]byte(`<meta name="tstamps-legal" content="1"></head>`), 1)
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(out)
				return
			}
			r.URL.Path = "/" + strings.TrimSuffix(name, "index.html")
		} else {
			if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "docs/assets/") || strings.HasPrefix(name, "screenshots/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = "/" + name
		}
		files.ServeHTTP(w, r)
	})
}
