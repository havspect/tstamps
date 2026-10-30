package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func TestAuditDateFormats(t *testing.T) {
	handler, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "acme"}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	admin, authHeader := seedToken(t, deps, ws.ID, "admin@acme.com", true)

	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	for i, day := range []string{"2026-09-30", "2026-10-15", "2026-11-01"} {
		at, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatalf("parse %s: %v", day, err)
		}
		log := &gormrepo.AuditLog{
			WorkspaceID: ws.ID,
			At:          at,
			ActorUserID: admin.ID,
			ActorRole:   "admin",
			Action:      "test.action",
			TargetType:  "entry",
			TargetID:    uint(i + 1),
			Changes:     `{}`,
		}
		if err := deps.DB.WithContext(ctxWs).Create(log).Error; err != nil {
			t.Fatalf("create audit log %s: %v", day, err)
		}
	}

	query := func(params string) (int, []auditEventResponse) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit"+params, nil)
		req.Header.Set("Authorization", authHeader)
		handler.ServeHTTP(rec, req)
		var out struct {
			Events []auditEventResponse `json:"events"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&out)
		return rec.Code, out.Events
	}

	assertDays := func(name string, params string, want ...string) {
		t.Run(name, func(t *testing.T) {
			code, events := query(params)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			if len(events) != len(want) {
				t.Fatalf("got %d events, want %d", len(events), len(want))
			}
			for i, e := range events {
				if got := e.At.UTC().Format("2006-01-02"); got != want[i] {
					t.Errorf("event %d at %s, want %s", i, got, want[i])
				}
			}
		})
	}

	assertDays("date range", "?from=2026-10-01&to=2026-11-01", "2026-10-15")
	assertDays("rfc3339 range", "?from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z", "2026-10-15")
	assertDays("no bounds", "", "2026-11-01", "2026-10-15", "2026-09-30")

	t.Run("invalid from", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?from=2026-13-01", nil)
		req.Header.Set("Authorization", authHeader)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422, body: %s", rec.Code, rec.Body.String())
		}
	})
}
