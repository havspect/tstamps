package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
