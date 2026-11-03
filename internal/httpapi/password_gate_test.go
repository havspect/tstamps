package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

func TestPasswordChangeGate(t *testing.T) {
	handler, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "acme"}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	user, token := seedToken(t, deps, ws.ID, "temp@acme.com", false)
	if err := deps.DB.WithContext(ctxWs).Model(&gormrepo.User{}).Where("id = ?", user.ID).
		Update("must_change_password", true).Error; err != nil {
		t.Fatalf("set must_change_password: %v", err)
	}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", token)
		handler.ServeHTTP(rec, req)
		return rec
	}

	// GET /me stays reachable and reports the flag.
	rec := call(http.MethodGet, "/api/v1/me", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me status=%d: %s", rec.Code, rec.Body.String())
	}
	var me MeResponse
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if !me.MustChangePassword {
		t.Fatalf("me must_change_password = false, want true")
	}

	// Only the allowlisted endpoints pass.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/me/tokens", `{"name":"x"}`},
		{http.MethodGet, "/api/v1/projects", ""},
		{http.MethodGet, "/api/v1/entries", ""},
		{http.MethodGet, "/api/v1/reports", ""},
	} {
		rec := call(tc.method, tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, want 403: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		var prob struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
			t.Fatalf("%s %s decode problem: %v", tc.method, tc.path, err)
		}
		if prob.Type != urnPasswordChangeRequired {
			t.Errorf("%s %s type=%q, want %q", tc.method, tc.path, prob.Type, urnPasswordChangeRequired)
		}
	}

	// The session path is gated the same way.
	loginRec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"temp@acme.com","password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d: %s", loginRec.Code, loginRec.Body.String())
	}
	var sessCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == sessionCookie {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("session cookie not found in login response")
	}
	sessRec := httptest.NewRecorder()
	sessReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	sessReq.AddCookie(sessCookie)
	handler.ServeHTTP(sessRec, sessReq)
	if sessRec.Code != http.StatusForbidden {
		t.Fatalf("session projects status=%d, want 403: %s", sessRec.Code, sessRec.Body.String())
	}

	// Changing the password lifts the restriction. The acting session survives
	// the change; the bearer token used before it does not.
	changeRec := httptest.NewRecorder()
	changeReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/password",
		strings.NewReader(`{"current_password":"secret123","new_password":"newsecret123"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.Header.Set("Origin", "http://example.com")
	changeReq.AddCookie(sessCookie)
	handler.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusNoContent {
		t.Fatalf("change password status=%d: %s", changeRec.Code, changeRec.Body.String())
	}
	afterRec := httptest.NewRecorder()
	afterReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	afterReq.AddCookie(sessCookie)
	handler.ServeHTTP(afterRec, afterReq)
	if afterRec.Code != http.StatusOK {
		t.Fatalf("projects after change status=%d: %s", afterRec.Code, afterRec.Body.String())
	}
}

func TestPasswordGateFailsClosed(t *testing.T) {
	_, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "acme"}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	user, _ := seedToken(t, deps, ws.ID, "member@acme.com", false)
	p := &auth.Principal{WorkspaceID: ws.ID, UserID: user.ID}

	// A handle without a workspace cannot read the flag, so the user is treated
	// as restricted rather than trusted.
	if !principalHasPasswordChangeRequired(p, deps.DB) {
		t.Errorf("unbound handle: got false, want true")
	}
	if principalHasPasswordChangeRequired(p, deps.DB.WithContext(ctxWs)) {
		t.Errorf("flag false: got true, want false")
	}
}
