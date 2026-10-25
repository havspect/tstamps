// Package api wires the HTTP surface: Huma operations over net/http, the session
// middleware, the generated OpenAPI contract, and /healthz.
package api

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/store"
	"github.com/havspect/tstamps/internal/version"
	"github.com/havspect/tstamps/web"
)

const (
	apiPrefix   = "/api/v1"
	openAPIPath = apiPrefix + "/openapi"
)

// Server is the state the handlers share: the store, the entry service, the
// injected clock, and the in-memory login limiter.
type Server struct {
	Store   *store.Store
	Clock   clock.Clock
	entries *entry.Service
	limit   *limiter
}

// New returns the http.Handler for the whole API and web surface.
func New(st *store.Store, clk clock.Clock) http.Handler {
	apiMux := http.NewServeMux()

	cfg := huma.DefaultConfig("Tstamps", version.Version)
	cfg.OpenAPIPath = openAPIPath
	cfg.DocsPath = apiPrefix + "/docs"
	cfg.SchemasPath = ""
	// The default hook injects $schema and a Link header into every response body.
	cfg.CreateHooks = nil
	api := humago.New(apiMux, cfg)

	// The document advertises both ways to authenticate; withRequest enforces them.
	doc := api.OpenAPI()
	doc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type: "http", Scheme: "bearer", BearerFormat: "ts_",
			Description: "Personal API token, shown once when it is created.",
		},
		"sessionCookie": {
			Type: "apiKey", In: "cookie", Name: sessionCookie,
			Description: "Session cookie set by POST /auth/login.",
		},
	}
	doc.Security = []map[string][]string{{"bearerAuth": {}}, {"sessionCookie": {}}}

	s := &Server{Store: st, Clock: clk, entries: entry.New(st, clk), limit: newLimiter(loginLimit, loginWindow)}
	s.auditRoutes(api)
	s.authRoutes(api)
	s.entryRoutes(api)
	s.projectRoutes(api)
	s.reportRoutes(api)
	s.teamRoutes(api)
	s.timerRoutes(api)
	s.tokenRoutes(api)
	s.userRoutes(api)

	// An unknown path under /api/ is still an API client: answer in the same
	// problem+json shape rather than net/http's plain-text 404.
	apiMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, problem(http.StatusNotFound, urnNotFound, "not found"))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	// The origin is per install, so these are built per request rather than
	// checked in as static files.
	mux.HandleFunc("GET /robots.txt", robots)
	mux.HandleFunc("GET /sitemap.xml", sitemap)
	// Only the API subtree needs a session: the SPA shell must load so a user
	// with must_change_password can reach the password screen.
	mux.Handle("/api/", s.withAccessLog(s.withRequest(apiMux)))
	mux.Handle("/", web.Handler())
	return SecurityHeaders(mux)
}

// origin is the scheme://host a crawler reached us on. It trusts the proxy's
// X-Forwarded-Proto only from a loopback peer, like the session cookie does.
func origin(r *http.Request) string {
	if isHTTPS(r) {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

// robots points crawlers at the per-install sitemap and keeps them out of
// everything behind the login.
func robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /login\nDisallow: /password\nDisallow: /signup\nDisallow: /timeline\nDisallow: /reports\nDisallow: /admin/\nDisallow: /api/\n\nSitemap: %s/sitemap.xml\n", origin(r))
}

// sitemap lists the only indexable page: the public homepage.
func sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = fmt.Fprintf(w, xml.Header+`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>%s/</loc></url></urlset>`+"\n", html.EscapeString(origin(r)))
}

// Spec returns the generated OpenAPI document, for `tstamps openapi`.
func Spec() ([]byte, error) {
	rec := httptest.NewRecorder()
	New(nil, clock.System{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, openAPIPath+".json", nil))
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("openapi: status %d", rec.Code)
	}
	return rec.Body.Bytes(), nil
}

// health answers 200 only when the database is reachable, and reports the schema
// version stored in the file rather than the one this binary expects.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	version, err := s.Store.SchemaVersion()
	if err == nil {
		err = s.Store.Ping()
	}
	if err != nil {
		slog.Error("health", "error", err)
		writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "the database is not reachable"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// Nothing to do if the write fails: the status and headers are already sent.
	_ = json.NewEncoder(w).Encode(struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}{"ok", version})
}

// SecurityHeaders are set here rather than in the proxy so the binary alone is
// still safe; HSTS stays with TLS termination.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		csp := "default-src 'self'; connect-src 'self' data:; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
		if r.URL.Path == "/docs" || strings.HasPrefix(r.URL.Path, "/docs/") {
			// VitePress ships inline page data and needs it to route. The docs tree
			// is built from this repo, not user input.
			csp = "default-src 'self'; script-src 'self' 'unsafe-inline'; connect-src 'self' data:; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
		}
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
