//go:build !web

// Package web answers with a note until the UI is built and embedded.
package web

import "net/http"

const notBuilt = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Tstamps</title></head>
<body><p>Tstamps UI is not built. Run <code>just build</code>.</p></body>
</html>
`

// Handler answers every path with the not-built page. The API is mounted
// separately, so this never sees /api requests.
func Handler() http.Handler { return notBuiltHandler() }

// HostedHandler has no embedded shell to mark; the host still routes normally.
func HostedHandler() http.Handler { return notBuiltHandler() }

func notBuiltHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(notBuilt))
	})
}
