package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func TestLogoutDeletesSession(t *testing.T) {
	handler, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "logout", CreatedAt: time.Now()}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	hash, _ := auth.HashPassword("secretpass123")
	u := &gormrepo.User{
		WorkspaceID:  ws.ID,
		Email:        "admin@logout.com",
		Name:         "Admin",
		PasswordHash: hash,
		Timezone:     "UTC",
		IsAdmin:      true,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := deps.DB.WithContext(ctxWs).Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@logout.com","password":"secretpass123"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie not found")
	}

	// A failed delete must not clear the cookie: the session row is still live.
	if err := deps.DB.Exec(`CREATE TRIGGER fail_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'forced'); END;`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "http://example.com")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("logout with failing delete status = %d, want 500, body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("failed logout cleared the cookie: %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me after failed logout = %d, want 200", rec.Code)
	}

	if err := deps.DB.Exec(`DROP TRIGGER fail_session_delete`).Error; err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "http://example.com")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204, body: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed cookie after logout = %d, want 401", rec.Code)
	}

	var sessions int64
	if err := deps.DB.WithContext(ctxWs).Model(&gormrepo.Session{}).Count(&sessions).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("sessions after logout = %d, want 0", sessions)
	}
}
