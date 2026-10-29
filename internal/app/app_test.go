package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildServer(t *testing.T) {
	cfg := Config{
		DB: ":memory:",
	}
	handler, db, err := BuildServer(cfg)
	if err != nil {
		t.Fatalf("BuildServer error: %v", err)
	}
	if handler == nil {
		t.Fatalf("expected non-nil handler")
	}
	if db != nil {
		sqlDB, err := db.DB()
		if err == nil {
			defer sqlDB.Close()
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}

	var health struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != "ok" || health.SchemaVersion != 8 {
		t.Errorf("health = %+v, want status ok and schema_version 8", health)
	}
}

func TestDemoServerSeedsAndLogsIn(t *testing.T) {
	handler, db, err := BuildServer(Config{DB: filepath.Join(t.TempDir(), "demo.db"), Demo: true})
	if err != nil {
		t.Fatalf("BuildServer: %v", err)
	}
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			defer sqlDB.Close()
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/w/demo/api/v1/auth/login",
		strings.NewReader(`{"email":"ada@example.com","password":"demo"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("demo login status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_admin":true`) {
		t.Errorf("demo admin login did not return an admin: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Disallow: /\n") {
		t.Errorf("demo robots.txt does not keep crawlers out: %s", rec.Body.String())
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg := Config{
		Addr: "127.0.0.1:0", // OS allocates a random available port
		DB:   ":memory:",
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(ctx, cfg)
	}()

	// Give server time to bind and listen
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned error on shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("server shutdown timed out")
	}
}
