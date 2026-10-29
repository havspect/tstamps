package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLegalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "imprint.html"), []byte("<h1>Impressum</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: Dependencies{LegalDir: dir}}

	rec := httptest.NewRecorder()
	s.legalFile("imprint.html").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/imprint", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "<h1>Impressum</h1>" {
		t.Fatalf("body = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff = %q", got)
	}

	rec = httptest.NewRecorder()
	s.legalFile("privacy.html").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/privacy", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing page status = %d, want 404", rec.Code)
	}
}
