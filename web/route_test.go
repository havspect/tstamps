package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesDocsWithoutSPAFallback(t *testing.T) {
	sub := fstest.MapFS{
		"index.html":         {Data: []byte("app")},
		"docs/index.html":    {Data: []byte("docs home")},
		"docs/cli.html":      {Data: []byte("cli")},
		"docs/assets/app.js": {Data: []byte("docs js")},
	}

	tests := []struct {
		path   string
		status int
		body   string
		cache  string
		robots string
	}{
		{"/", http.StatusOK, "app", "no-store", ""},
		{"/docs", http.StatusOK, "docs home", "no-store", ""},
		{"/docs/", http.StatusOK, "docs home", "no-store", ""},
		{"/docs/cli", http.StatusOK, "cli", "", ""},
		{"/docs/cli/", http.StatusOK, "cli", "", ""},
		{"/docs/assets/app.js", http.StatusOK, "docs js", "public, max-age=31536000, immutable", ""},
		{"/timeline", http.StatusOK, "app", "no-store", "noindex, nofollow"},
		{"/login", http.StatusOK, "app", "no-store", "noindex, nofollow"},
		{"/docs/missing", http.StatusNotFound, "", "", ""},
		{"/assets/missing.js", http.StatusNotFound, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler(sub, false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tt.body)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.cache {
				t.Fatalf("cache = %q, want %q", got, tt.cache)
			}
			if got := rec.Header().Get("X-Robots-Tag"); got != tt.robots {
				t.Fatalf("robots = %q, want %q", got, tt.robots)
			}
		})
	}
}

func TestDemoAccountsAreStampedIntoTheShell(t *testing.T) {
	sub := fstest.MapFS{
		"index.html": {Data: []byte("<head></head>")},
	}

	rec := httptest.NewRecorder()
	handler(sub, true, Options{DemoAccounts: `[{"email":"ada@example.com","password":"demo"}]`}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `name="tstamps-demo"`) || !strings.Contains(body, "ada@example.com") {
		t.Fatalf("demo shell = %q, want the demo marker with its accounts", body)
	}
	if !strings.Contains(body, `name="tstamps-hosted"`) {
		t.Fatalf("demo shell lost the hosted marker: %q", body)
	}

	rec = httptest.NewRecorder()
	handler(sub, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), "tstamps-demo") {
		t.Fatalf("plain shell carries the demo marker: %q", rec.Body.String())
	}
}

func TestHostedHandlerMarksOnlyTheShell(t *testing.T) {
	sub := fstest.MapFS{
		"index.html":      {Data: []byte("<head></head>")},
		"docs/index.html": {Data: []byte("docs home")},
	}

	rec := httptest.NewRecorder()
	handler(sub, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), `<meta name="tstamps-hosted" content="1">`) {
		t.Fatalf("hosted shell = %q, want the hosted marker", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler(sub, false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), "tstamps-hosted") {
		t.Fatalf("one-workspace shell carries the hosted marker: %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler(sub, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Body.String() != "docs home" {
		t.Fatalf("hosted docs = %q, want the manual, not the marked shell", rec.Body.String())
	}
}

func TestHandlerReplacesOriginInIndex(t *testing.T) {
	sub := fstest.MapFS{
		"index.html": {Data: []byte(`<head><link rel="canonical" href="/" /><meta property="og:url" content="/" /><meta property="og:image" content="/screenshots/timeline.webp" /><meta name="twitter:image" content="/screenshots/timeline.webp" /></head>`)},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "time.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	handler(sub, false).ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `<link rel="canonical" href="https://time.example.com/"`) {
		t.Fatalf("canonical href not replaced: %s", body)
	}
	if !strings.Contains(body, `<meta property="og:url" content="https://time.example.com/"`) {
		t.Fatalf("og:url content not replaced: %s", body)
	}
	if !strings.Contains(body, `content="https://time.example.com/screenshots/timeline.webp"`) {
		t.Fatalf("og:image content not replaced: %s", body)
	}
}

func TestHandlerCachesScreenshots(t *testing.T) {
	sub := fstest.MapFS{
		"screenshots/timeline.webp": {Data: []byte("img")},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/screenshots/timeline.webp", nil)
	handler(sub, false).ServeHTTP(rec, req)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("screenshot cache = %q, want immutable", got)
	}
}

func TestHostedHandlerMarksConfiguredLegalPages(t *testing.T) {
	sub := fstest.MapFS{
		"index.html": {Data: []byte("<head></head>")},
	}

	rec := httptest.NewRecorder()
	handler(sub, true, Options{Legal: true}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), `<meta name="tstamps-legal" content="1">`) {
		t.Fatalf("shell without the legal marker: %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler(sub, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), "tstamps-legal") {
		t.Fatalf("unconfigured instance carries the legal marker: %q", rec.Body.String())
	}
}
