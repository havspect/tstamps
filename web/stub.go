//go:build !web

// Package web answers with a note until the UI is built and embedded.
package web

import "net/http"

const notBuilt = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Tstamps</title></head>
<body style="font-family: system-ui, sans-serif; padding: 2rem; line-height: 1.5;">
  <h2>Tstamps UI is not embedded in this server binary</h2>
  <p>This process was started without <code>-tags web</code>.</p>
  <ul>
    <li>To serve the embedded UI and manual: build and run the server binary:
      <pre>just build && ./tstamps-server serve</pre>
      (or <code>just serve</code>)
    </li>
    <li>If you are running the development stack (<code>just dev</code>):
      <ul>
        <li>Timeline UI: <a href="http://localhost:5173">http://localhost:5173</a></li>
        <li>Manual / Docs: <a href="http://localhost:5174/docs/">http://localhost:5174/docs/</a></li>
      </ul>
    </li>
  </ul>
</body>
</html>
`

// Handler answers every path with the not-built page. The API is mounted
// separately, so this never sees /api requests.
func Handler() http.Handler { return notBuiltHandler() }

// HostedHandler has no embedded shell to mark; the host still routes normally.
func HostedHandler(bool) http.Handler { return notBuiltHandler() }

func notBuiltHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(notBuilt))
	})
}
