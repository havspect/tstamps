// Package httpapi wires the HTTP surface: Huma v2 OpenAPI operations over net/http.ServeMux,
// edge middleware (SecurityHeaders, withAccessLog, withAuth), health checks, OAuth, MCP, and the Web SPA.
package httpapi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/oauth"
	"github.com/havspect/tstamps/internal/onboarding"
	"github.com/havspect/tstamps/internal/plan"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
	"github.com/havspect/tstamps/internal/version"
	"github.com/havspect/tstamps/web"
)

const (
	apiPrefix   = "/api/v1"
	openAPIPath = apiPrefix + "/openapi"
)

// Dependencies bundles the composition root inputs needed by the HTTP layer.
type Dependencies struct {
	DB                *gorm.DB
	Clock             clock.Clock
	UserService       *user.Service
	ProjectService    *project.Service
	TeamService       *team.Service
	EntryService      *entry.Service
	ReportService     *report.Service
	PlanService       *plan.Service
	AuditService      *audit.Service
	OnboardingService *onboarding.Service
	OAuthProvider     *oauth.Provider
	MCPHandler        http.Handler
	DevMode           bool
}

// Server holds state shared across HTTP handlers.
type Server struct {
	deps Dependencies
}

// New returns the top-level http.Handler for the HTTP surface.
func New(deps Dependencies) http.Handler {
	if deps.Clock == nil {
		deps.Clock = clock.System{}
	}

	s := &Server{deps: deps}

	apiMux := http.NewServeMux()

	cfg := huma.DefaultConfig("Tstamps", version.Version)
	cfg.OpenAPIPath = openAPIPath
	cfg.DocsPath = apiPrefix + "/docs"
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	api := humago.New(apiMux, cfg)

	doc := api.OpenAPI()
	doc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "ts_",
			Description:  "Personal API token or OAuth access token.",
		},
		"sessionCookie": {
			Type:        "apiKey",
			In:          "cookie",
			Name:        sessionCookie,
			Description: "Session cookie set by POST /api/v1/auth/login.",
		},
	}
	doc.Security = []map[string][]string{{"bearerAuth": {}}, {"sessionCookie": {}}}

	// Register domain feature routes
	s.authRoutes(api)
	s.userRoutes(api)
	s.tokenRoutes(api)
	s.teamRoutes(api)
	s.projectRoutes(api)
	s.entryRoutes(api)
	s.timerRoutes(api)
	s.reportRoutes(api)
	s.planRoutes(api)
	s.auditRoutes(api)
	s.signupRoutes(api)

	// API Mux unknown route fallback in RFC 9457 format
	apiMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, problem(http.StatusNotFound, urnNotFound, "not found"))
	})

	topMux := http.NewServeMux()

	// Health check
	topMux.HandleFunc("GET /healthz", s.health)

	// Crawler rules & sitemap
	topMux.HandleFunc("GET /robots.txt", s.robots)
	topMux.HandleFunc("GET /sitemap.xml", s.sitemap)

	// OAuth discovery & routes
	s.registerOAuthRoutes(topMux)

	// MCP Streamable HTTP handler
	if deps.MCPHandler != nil {
		topMux.Handle("/mcp", s.withAccessLog(s.withAuth(deps.MCPHandler)))
		topMux.Handle("/api/v1/mcp", s.withAccessLog(s.withAuth(deps.MCPHandler)))
		apiMux.Handle("/mcp", deps.MCPHandler)
	}

	// API routes subtree
	topMux.Handle("/api/", s.withAccessLog(s.withAuth(apiMux)))

	// Web SPA: dev reverse-proxy or embedded assets
	if deps.DevMode {
		devURL, _ := url.Parse("http://localhost:5173")
		topMux.Handle("/", httputil.NewSingleHostReverseProxy(devURL))
	} else {
		topMux.Handle("/", web.HostedHandler())
	}

	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Well-known OAuth with workspace
		if strings.HasPrefix(r.URL.Path, "/.well-known/oauth-authorization-server/w/") {
			slugPrefix := "/.well-known/oauth-authorization-server/w/"
			slug := strings.TrimPrefix(r.URL.Path, slugPrefix)
			if idx := strings.Index(slug, "/"); idx != -1 {
				slug = slug[:idx]
			}
			if slug != "" && deps.DB != nil {
				var ws gormrepo.Workspace
				if err := deps.DB.Where("slug = ?", slug).First(&ws).Error; err == nil {
					r2 := r.Clone(gormrepo.WithWorkspaceID(r.Context(), ws.ID))
					r2.URL.Path = "/.well-known/oauth-authorization-server"
					topMux.ServeHTTP(w, r2)
					return
				}
			}
		}
		if strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource/w/") {
			slugPrefix := "/.well-known/oauth-protected-resource/w/"
			slug := strings.TrimPrefix(r.URL.Path, slugPrefix)
			if idx := strings.Index(slug, "/"); idx != -1 {
				slug = slug[:idx]
			}
			if slug != "" && deps.DB != nil {
				var ws gormrepo.Workspace
				if err := deps.DB.Where("slug = ?", slug).First(&ws).Error; err == nil {
					r2 := r.Clone(gormrepo.WithWorkspaceID(r.Context(), ws.ID))
					r2.URL.Path = "/.well-known/oauth-protected-resource"
					topMux.ServeHTTP(w, r2)
					return
				}
			}
		}

		if r.URL.Path == "/api/v1/workspace" {
			slug := ""
			if deps.DB != nil {
				var workspaces []gormrepo.Workspace
				_ = deps.DB.Order("id ASC").Find(&workspaces).Error
				if len(workspaces) == 1 {
					slug = workspaces[0].Slug
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"slug": slug})
			return
		}

		if r.URL.Path == "/login" && deps.DB != nil {
			var workspaces []gormrepo.Workspace
			_ = deps.DB.Order("id ASC").Find(&workspaces).Error
			if len(workspaces) == 1 {
				http.Redirect(w, r, "/w/"+workspaces[0].Slug+"/login", http.StatusFound)
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/w/") {
			slug, after, found := strings.Cut(strings.TrimPrefix(r.URL.Path, "/w/"), "/")
			rest := "/"
			if found {
				rest = "/" + after
			}
			if err := onboarding.ValidateSlug(slug); err != nil {
				s.workspaceNotFound(w, rest)
				return
			}
			if deps.DB == nil {
				s.workspaceNotFound(w, rest)
				return
			}
			var ws gormrepo.Workspace
			if err := deps.DB.Where("slug = ?", slug).First(&ws).Error; err != nil {
				s.workspaceNotFound(w, rest)
				return
			}

			r2 := r.Clone(gormrepo.WithWorkspaceID(r.Context(), ws.ID))
			r2.URL.Path = rest
			topMux.ServeHTTP(w, r2)
			return
		}

		topMux.ServeHTTP(w, r)
	})

	return SecurityHeaders(rootHandler)
}

func (s *Server) workspaceNotFound(w http.ResponseWriter, rest string) {
	if strings.HasPrefix(rest, "/api/") || strings.HasPrefix(rest, "/.well-known/") || strings.HasPrefix(rest, "/oauth/") || strings.HasPrefix(rest, "/mcp") {
		writeProblem(w, problem(http.StatusNotFound, urnNotFound, "workspace not found"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Workspace not found</title><h1>Workspace not found</h1>"))
}

// Spec returns the generated OpenAPI document as raw bytes.
func Spec() ([]byte, error) {
	rec := httptest.NewRecorder()
	h := New(Dependencies{})
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, openAPIPath+".json", nil))
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("openapi: status %d", rec.Code)
	}
	return rec.Body.Bytes(), nil
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if s.deps.DB == nil {
		writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "the database is not configured"))
		return
	}

	sqlDB, err := s.deps.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		writeProblem(w, problem(http.StatusServiceUnavailable, urnUnavailable, "the database is not reachable"))
		return
	}

	version := 8
	var pragmaVersion int
	if err := s.deps.DB.Raw("PRAGMA user_version").Scan(&pragmaVersion).Error; err == nil && pragmaVersion > 0 {
		version = pragmaVersion
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}{
		Status:        "ok",
		SchemaVersion: version,
	})
}

func (s *Server) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nAllow: /docs/\nDisallow: /login\nDisallow: /password\nDisallow: /signup\nDisallow: /timeline\nDisallow: /reports\nDisallow: /plan\nDisallow: /admin/\nDisallow: /api/\n\nSitemap: %s/sitemap.xml\n", origin(r))
}

func (s *Server) sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	base := html.EscapeString(origin(r))
	docs := []string{
		"/",
		"/docs/",
		"/docs/install-cli",
		"/docs/timeline",
		"/docs/reports",
		"/docs/plan",
		"/docs/cli",
		"/docs/mcp",
		"/docs/agents/",
		"/docs/agents/claude",
		"/docs/agents/codex",
		"/docs/agents/langdock",
		"/docs/agents/opencode",
		"/docs/admin",
		"/docs/server",
		"/docs/workspaces",
		"/docs/backup-upgrade",
	}
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, doc := range docs {
		fmt.Fprintf(&b, "  <url><loc>%s%s</loc></url>\n", base, doc)
	}
	b.WriteString("</urlset>\n")
	_, _ = w.Write([]byte(b.String()))
}
