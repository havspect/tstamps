package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
	"github.com/havspect/tstamps/web"
)

// ErrClosed is returned once the host has started shutting down.
var ErrClosed = errors.New("workspace host closed")

// Host routes /w/{slug}/... to a cached API handler for that workspace's own
// SQLite database. Tenant selection happens before authentication, so a session
// or token is only ever looked up in the workspace named by the URL.
type Host struct {
	dir   string
	clock clock.Clock
	web   http.Handler
	serve http.Handler

	mu     sync.Mutex
	cache  map[string]*instance
	closed bool
}

type instance struct {
	ready   chan struct{}
	handler http.Handler
	store   *store.Store
	err     error
}

// New serves the workspace databases in dir. The directory must already exist;
// New never creates a workspace.
func New(dir string, clk clock.Clock) (*Host, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	h := &Host{
		dir: dir, clock: clk, web: web.HostedHandler(),
		cache: make(map[string]*instance),
	}
	h.serve = api.SecurityHeaders(http.HandlerFunc(h.route))
	return h, nil
}

func (h *Host) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serve.ServeHTTP(w, r)
}

// Close releases every cached workspace database. It waits for an in-flight
// first request to finish opening its store.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	cached := h.cache
	h.cache = make(map[string]*instance)
	h.mu.Unlock()

	var errs []error
	for _, inst := range cached {
		<-inst.ready
		if inst.store != nil {
			if err := inst.store.Close(); err != nil {
				errs = append(errs, err)
			}
			inst.store = nil
		}
	}
	return errors.Join(errs...)
}

func (h *Host) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		h.health(w)
		return
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

	inst, err := h.instance(slug)
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
	inst.handler.ServeHTTP(w, r)
}

func (h *Host) instance(slug string) (*instance, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrClosed
	}
	if inst, ok := h.cache[slug]; ok {
		h.mu.Unlock()
		<-inst.ready
		return inst, inst.err
	}
	inst := &instance{ready: make(chan struct{})}
	h.cache[slug] = inst
	h.mu.Unlock()

	path, err := DatabasePath(h.dir, slug)
	var st *store.Store
	var handler http.Handler
	if err == nil {
		_, err = os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			err = ErrNotFound
		}
	}
	if err == nil {
		st, err = store.Open(path)
	}
	if err == nil {
		err = st.Migrate()
	}
	if err == nil {
		handler = api.New(st, h.clock)
	}
	if err != nil && st != nil {
		_ = st.Close()
		st = nil
	}

	h.mu.Lock()
	if err == nil && h.closed {
		err = ErrClosed
	}
	if err != nil && st != nil {
		_ = st.Close()
		st = nil
	}
	inst.store, inst.handler, inst.err = st, handler, err
	close(inst.ready)
	if err != nil {
		delete(h.cache, slug)
	}
	h.mu.Unlock()
	return inst, err
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
	if strings.HasPrefix(rest, "/api/") {
		api.WriteNotFound(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Workspace not found</title><h1>Workspace not found</h1>"))
}

func (h *Host) health(w http.ResponseWriter) {
	if _, err := os.ReadDir(h.dir); err != nil {
		slog.Error("health", "error", err)
		api.WriteUnavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{"ok"})
}
