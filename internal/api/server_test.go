package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

func handler() http.Handler { return New(nil, clock.System{}) }

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q, want application/json", ct)
	}
	var body struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" || body.SchemaVersion != store.SchemaVersion {
		t.Fatalf("body = %+v, want status ok and schema_version %d", body, store.SchemaVersion)
	}
}

func TestHealthzFailsWhenTheDatabaseIsGone(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tstamps.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	h := New(st, clock.System{})
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	wantProblem(t, rec, http.StatusServiceUnavailable, urnUnavailable)
}

func TestSecurityHeaders(t *testing.T) {
	for _, path := range []string{"/", "/api/v1/unknown"} {
		rec := get(t, path)
		if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
			t.Errorf("%s CSP = %q, want a default-src 'self' policy", path, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := rec.Header().Get("Referrer-Policy"); got != "same-origin" {
			t.Errorf("%s Referrer-Policy = %q, want same-origin", path, got)
		}
	}
}

func TestSecurityHeadersAllowInlineDocsData(t *testing.T) {
	rec := get(t, "/docs/")
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("/docs/ CSP = %q, want inline scripts allowed", got)
	}
	rec = get(t, "/api/v1/unknown")
	if got := rec.Header().Get("Content-Security-Policy"); strings.Contains(got, "script-src") {
		t.Errorf("/api/ CSP = %q, must not allow inline scripts", got)
	}
}

type openAPI struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Title string `json:"title"`
	} `json:"info"`
}

func TestOpenAPI(t *testing.T) {
	rec := get(t, "/api/v1/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var doc openAPI
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.1") {
		t.Fatalf("openapi = %q, want 3.1.x", doc.OpenAPI)
	}
	if doc.Info.Title != "Tstamps" {
		t.Fatalf("info.title = %q, want Tstamps", doc.Info.Title)
	}
}

func TestDocs(t *testing.T) {
	rec := get(t, "/api/v1/docs")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type = %q, want text/html", ct)
	}
}

func TestSpecMatchesRoute(t *testing.T) {
	spec, err := Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if want := get(t, "/api/v1/openapi.json").Body.String(); string(spec) != want {
		t.Fatalf("Spec differs from route:\n%s\nwant:\n%s", spec, want)
	}
}

func TestUnknownAPIPathIsProblemJSON(t *testing.T) {
	rec := get(t, "/api/v1/unknown")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type = %q, want application/problem+json", ct)
	}
}

func TestSitemapAndRobots(t *testing.T) {
	sitemap := get(t, "/sitemap.xml")
	if sitemap.Code != http.StatusOK {
		t.Fatalf("sitemap status = %d, want 200", sitemap.Code)
	}
	if ct := sitemap.Header().Get("Content-Type"); ct != "application/xml" {
		t.Fatalf("sitemap content type = %q, want application/xml", ct)
	}
	body := sitemap.Body.String()
	if !strings.Contains(body, `<loc>http://example.com/</loc>`) {
		t.Fatalf("sitemap = %q, want the request origin", body)
	}
	if strings.Contains(body, "/timeline") {
		t.Fatalf("sitemap = %q, want only the indexable homepage", body)
	}

	robots := get(t, "/robots.txt")
	if robots.Code != http.StatusOK {
		t.Fatalf("robots status = %d, want 200", robots.Code)
	}
	if ct := robots.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("robots content type = %q, want text/plain", ct)
	}
	lines := robots.Body.String()
	for _, want := range []string{"Allow: /", "Disallow: /api/", "Sitemap: http://example.com/sitemap.xml"} {
		if !strings.Contains(lines, want) {
			t.Errorf("robots.txt missing %q:\n%s", want, lines)
		}
	}
}

func TestSitemapUsesTheForwardedScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler().ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, `<loc>https://example.com/</loc>`) {
		t.Fatalf("sitemap = %q, want an https location", body)
	}
}

func TestWebServesTheAppShell(t *testing.T) {
	// The stub build answers every non-API path with the same page; the embedded
	// build answers index.html for both. Either way the shell must load.
	for _, path := range []string{"/", "/timeline"} {
		rec := get(t, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s content type = %q, want text/html", path, ct)
		}
	}
}

func TestMustChangePasswordLoadsTheAppButNotTheAPI(t *testing.T) {
	h := newHarness(t)
	hash, err := auth.HashPassword("initial password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &store.User{
		Email: "new@example.com", Name: "New Person", PasswordHash: hash,
		Timezone: "UTC", Active: true, MustChangePassword: true,
	}
	if err := h.st.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	c := h.as(t, u)
	if rec := h.do(http.MethodGet, "/", nil, nil, c); rec.Code != http.StatusOK {
		t.Fatalf("shell status = %d, want 200: %s", rec.Code, rec.Body)
	}
	wantProblem(t, h.do(http.MethodGet, apiPrefix+"/entries", nil, nil, c),
		http.StatusForbidden, urnPasswordChangeRequired)
}
