package workspace

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
	"github.com/havspect/tstamps/web"
)

// ErrClosed is returned once the host has started shutting down.
var ErrClosed = errors.New("workspace host closed")

// Host routes /w/{slug}/... to a cached API handler for that workspace. Tenant
// selection happens before authentication; the handler only sees a Store scoped
// to the workspace named by the URL.
type Host struct {
	reg    *store.Registry
	clock  clock.Clock
	web    http.Handler
	signup http.Handler
	serve  http.Handler

	mu     sync.Mutex
	cache  map[string]http.Handler
	closed bool
}

// New serves the one database at db, migrating it once at boot. signup answers
// the host-level signup routes and may be nil, which makes them answer "not
// enabled".
func New(db string, clk clock.Clock, signup http.Handler) (*Host, error) {
	reg, err := store.OpenRegistry(db)
	if err != nil {
		return nil, err
	}
	if err := reg.Migrate(); err != nil {
		_ = reg.Close()
		return nil, err
	}
	if signup == nil {
		signup = api.NewSignupHandler(nil, clk)
	}
	h := &Host{
		reg: reg, clock: clk, web: web.HostedHandler(), signup: signup,
		cache: make(map[string]http.Handler),
	}
	h.serve = api.SecurityHeaders(http.HandlerFunc(h.route))
	return h, nil
}

func (h *Host) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serve.ServeHTTP(w, r)
}

// Close releases the shared database. Cached handlers hold no separate handles.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.cache = map[string]http.Handler{}
	h.mu.Unlock()
	return h.reg.Close()
}

func (h *Host) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		h.health(w)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/.well-known/oauth-authorization-server/w/") {
		slugPrefix := "/.well-known/oauth-authorization-server/w/"
		slug := strings.TrimPrefix(r.URL.Path, slugPrefix)
		if idx := strings.Index(slug, "/"); idx != -1 {
			slug = slug[:idx]
		}
		if slug != "" {
			if handler, err := h.instance(slug); err == nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/.well-known/oauth-authorization-server"
				r = api.WithWorkspace(r, slug)
				handler.ServeHTTP(w, r)
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
		if slug != "" {
			if handler, err := h.instance(slug); err == nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/.well-known/oauth-protected-resource"
				r = api.WithWorkspace(r, slug)
				handler.ServeHTTP(w, r)
				return
			}
		}
	}
	if r.URL.Path == "/robots.txt" {
		api.Robots(w, r)
		return
	}
	if r.URL.Path == "/sitemap.xml" {
		api.Sitemap(w, r)
		return
	}
	// Signup happens before a workspace exists, so it is answered here rather
	// than by the per-workspace API.
	if strings.HasPrefix(r.URL.Path, "/api/v1/signup") {
		h.signup.ServeHTTP(w, r)
		return
	}
	// One workspace means one sign-in page: skip the slug prompt.
	if r.URL.Path == "/api/v1/workspace" {
		h.onlyWorkspaceJSON(w)
		return
	}
	if r.URL.Path == "/login" {
		if slug, ok, err := h.onlySlug(); err != nil {
			slog.Error("read workspaces", "error", err)
			api.WriteInternal(w)
			return
		} else if ok {
			http.Redirect(w, r, "/w/"+slug+"/login", http.StatusFound)
			return
		}
	}

	slug, rest, ok := splitWorkspace(r.URL.Path)
	if !ok {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.WriteNotFound(w)
			return
		}
		h.web.ServeHTTP(w, r)
		return
	}
	if ValidateSlug(slug) != nil {
		h.notFound(w, rest)
		return
	}

	handler, err := h.instance(slug)
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrInvalidSlug):
		h.notFound(w, rest)
		return
	case errors.Is(err, ErrClosed):
		api.WriteUnavailable(w)
		return
	case err != nil:
		slog.Error("open workspace", "workspace", slug, "error", err)
		api.WriteInternal(w)
		return
	}

	// The workspace handler sees the ordinary unprefixed paths, so every
	// existing route, permission check and password gate stays unchanged.
	r = r.Clone(r.Context())
	r.URL.Path = rest
	r = api.WithWorkspace(r, slug)
	handler.ServeHTTP(w, r)
}

func (h *Host) onlySlug() (string, bool, error) {
	workspaces, err := h.reg.Workspaces()
	if err != nil {
		return "", false, err
	}
	if len(workspaces) != 1 {
		return "", false, nil
	}
	return workspaces[0].Slug, true, nil
}

// onlyWorkspaceJSON tells the SPA the sole workspace's slug, so a client-side
// link to /login can skip the prompt. It never lists a multi-workspace server.
func (h *Host) onlyWorkspaceJSON(w http.ResponseWriter) {
	slug, ok, err := h.onlySlug()
	if err != nil {
		slog.Error("read workspaces", "error", err)
		api.WriteInternal(w)
		return
	}
	if !ok {
		slug = ""
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Slug string `json:"slug"`
	}{slug})
}

func (h *Host) instance(slug string) (http.Handler, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrClosed
	}
	if handler, ok := h.cache[slug]; ok {
		h.mu.Unlock()
		return handler, nil
	}
	h.mu.Unlock()

	ws, err := h.reg.WorkspaceBySlug(slug)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	handler := api.New(h.reg.ForWorkspace(ws.ID), h.clock)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	// Two first requests can race; either handler is equivalent, keep one.
	if cached, ok := h.cache[slug]; ok {
		return cached, nil
	}
	h.cache[slug] = handler
	return handler, nil
}

func splitWorkspace(path string) (slug, rest string, ok bool) {
	if !strings.HasPrefix(path, "/w/") {
		return "", "", false
	}
	slug, after, found := strings.Cut(strings.TrimPrefix(path, "/w/"), "/")
	if !found {
		return slug, "/", true
	}
	return slug, "/" + after, true
}

func (h *Host) notFound(w http.ResponseWriter, rest string) {
	if strings.HasPrefix(rest, "/api/") || strings.HasPrefix(rest, "/.well-known/") || strings.HasPrefix(rest, "/oauth/") || strings.HasPrefix(rest, "/mcp") {
		api.WriteNotFound(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Workspace not found</title><h1>Workspace not found</h1>"))
}

func (h *Host) health(w http.ResponseWriter) {
	if err := h.reg.Ping(); err != nil {
		slog.Error("health", "error", err)
		api.WriteUnavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{"ok"})
}
