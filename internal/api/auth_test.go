package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

var _ clock.Clock = (*fakeClock)(nil)

type harness struct {
	h        http.Handler
	st       *store.Store
	clock    *fakeClock
	user     *store.User
	password string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tstamps.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const password = "correct horse battery staple"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &store.User{
		Email: "ada@example.com", Name: "Ada Lovelace", PasswordHash: hash,
		Timezone: "UTC", IsAdmin: true, Active: true,
	}
	if err := st.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	clk := &fakeClock{now: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	return &harness{h: New(st, clk), st: st, clock: clk, user: u, password: password}
}

func (h *harness) request(method, path string, body any, headers map[string]string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func (h *harness) send(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}

func (h *harness) do(method, path string, body any, headers map[string]string, cookie string) *httptest.ResponseRecorder {
	req := h.request(method, path, body, headers)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	return h.send(req)
}

func (h *harness) login(email, password string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": email, "password": password}, nil, "")
}

func (h *harness) loginFrom(remoteAddr, email, password string) *httptest.ResponseRecorder {
	req := h.request(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": email, "password": password}, nil)
	req.RemoteAddr = remoteAddr
	return h.send(req)
}

func sessionOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range (&http.Response{Header: rec.Header()}).Cookies() {
		if c.Name == sessionCookie || c.Name == secureSessionCookie {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestLogin(t *testing.T) {
	h := newHarness(t)
	rec := h.login(h.user.Email, h.password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	me := decode[meResponse](t, rec)
	if me.Email != h.user.Email || me.Name != h.user.Name || !me.IsAdmin {
		t.Fatalf("body = %+v, want the user", me)
	}
	if me.MustChangePassword {
		t.Fatal("must_change_password = true for a user who set their own password")
	}

	c := sessionOf(t, rec)
	if c.Value == "" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("cookie = %+v, want HttpOnly Lax session on /", c)
	}
	if c.MaxAge != int(auth.SessionTTL/time.Second) {
		t.Fatalf("max-age = %d, want %d", c.MaxAge, int(auth.SessionTTL/time.Second))
	}
	if c.Secure {
		t.Fatal("cookie is Secure over plain http")
	}

	if _, err := h.st.SessionByTokenHash(auth.HashToken(c.Value)); err != nil {
		t.Fatalf("session not stored under the token hash: %v", err)
	}
	if _, err := h.st.SessionByTokenHash(c.Value); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("raw token lookup = %v, want ErrNotFound (never store the raw value)", err)
	}
}

func TestLoginSecureCookie(t *testing.T) {
	h := newHarness(t)
	https := map[string]string{"X-Forwarded-Proto": "https, http"}
	// The proxy connects from loopback, so its forwarded scheme is trusted.
	req := h.request(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": h.user.Email, "password": h.password}, https)
	req.RemoteAddr = "127.0.0.1:9000"
	rec := h.send(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	c := sessionOf(t, rec)
	if c.Name != secureSessionCookie || !c.Secure {
		t.Fatalf("cookie = %+v, want a Secure %s", c, secureSessionCookie)
	}

	// The middleware accepts the secure name and authenticates with it.
	req = h.request(http.MethodGet, "/api/v1/me", nil, https)
	req.RemoteAddr = "127.0.0.1:9000"
	req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	if rec := h.send(req); rec.Code != http.StatusOK {
		t.Fatalf("status with secure cookie = %d, want 200: %s", rec.Code, rec.Body)
	}
}

func TestLoginRejected(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		password   string
		deactivate bool
	}{
		{"wrong password", "ada@example.com", "wrong", false},
		{"unknown email", "nobody@example.com", "correct horse battery staple", false},
		{"inactive user", "ada@example.com", "correct horse battery staple", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			if tt.deactivate {
				if err := h.st.SetUserActive(h.user.ID, false); err != nil {
					t.Fatalf("deactivate: %v", err)
				}
			}
			rec := h.login(tt.email, tt.password)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
				t.Fatalf("content type = %q, want application/problem+json", ct)
			}
			body := decode[struct {
				Type   string `json:"type"`
				Status int    `json:"status"`
			}](t, rec)
			if body.Type != urnInvalidCredentials || body.Status != http.StatusUnauthorized {
				t.Fatalf("body = %+v, want %s", body, urnInvalidCredentials)
			}
		})
	}
}

func TestMe(t *testing.T) {
	h := newHarness(t)
	c := sessionOf(t, h.login(h.user.Email, h.password))

	rec := h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if me := decode[meResponse](t, rec); me.Email != h.user.Email {
		t.Fatalf("email = %q, want %q", me.Email, h.user.Email)
	}

	rec = h.do(http.MethodGet, "/api/v1/me", nil, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without cookie = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type = %q, want application/problem+json", ct)
	}
	if body := decode[struct {
		Type string `json:"type"`
	}](t, rec); body.Type != urnUnauthorized {
		t.Fatalf("type = %q, want %q", body.Type, urnUnauthorized)
	}

	h.clock.now = h.clock.now.Add(auth.SessionTTL + time.Second)
	rec = h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status with expired session = %d, want 401", rec.Code)
	}
	if clear := sessionOf(t, rec); clear.Value != "" || clear.MaxAge >= 0 {
		t.Fatalf("clearing cookie = %+v, want empty value and MaxAge < 0", clear)
	}
	if _, err := h.st.SessionByTokenHash(auth.HashToken(c.Value)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired session lookup = %v, want ErrNotFound (the row must be deleted)", err)
	}
}

func TestLogout(t *testing.T) {
	h := newHarness(t)
	c := sessionOf(t, h.login(h.user.Email, h.password))
	origin := map[string]string{"Origin": "http://example.com"}

	// A safe method needs no Origin.
	if rec := h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value); rec.Code != http.StatusOK {
		t.Fatalf("GET /me without Origin = %d, want 200", rec.Code)
	}

	rec := h.do(http.MethodPost, "/api/v1/auth/logout", nil, origin, c.Value)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body)
	}
	clear := sessionOf(t, rec)
	if clear.Value != "" || clear.MaxAge >= 0 {
		t.Fatalf("clearing cookie = %+v, want empty value and MaxAge < 0", clear)
	}
	if _, err := h.st.SessionByTokenHash(auth.HashToken(c.Value)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("session lookup = %v, want ErrNotFound after logout", err)
	}
	if rec = h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want 401", rec.Code)
	}

	c = sessionOf(t, h.login(h.user.Email, h.password))
	if rec = h.do(http.MethodPost, "/api/v1/auth/logout", nil, nil, c.Value); rec.Code != http.StatusForbidden {
		t.Fatalf("status without Origin = %d, want 403", rec.Code)
	} else if body := decode[struct {
		Type string `json:"type"`
	}](t, rec); body.Type != urnInvalidOrigin {
		t.Fatalf("type = %q, want %q", body.Type, urnInvalidOrigin)
	}

	if rec = h.do(http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{"Origin": "https://evil.example"}, c.Value); rec.Code != http.StatusForbidden {
		t.Fatalf("status with foreign Origin = %d, want 403", rec.Code)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	h := newHarness(t)
	if rec := h.do(http.MethodPost, "/api/v1/nope", nil, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (unauthenticated requests must not become 401)", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	const ip = "203.0.113.7:4444"
	for i := 0; i < loginLimit; i++ {
		if rec := h.loginFrom(ip, h.user.Email, h.password); rec.Code != http.StatusOK {
			t.Fatalf("attempt %d = %d, want 200 (a success still spends an attempt): %s", i+1, rec.Code, rec.Body)
		}
	}

	rec := h.loginFrom(ip, h.user.Email, h.password)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status over the limit = %d, want 429: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type = %q, want application/problem+json", ct)
	}
	if body := decode[struct {
		Type string `json:"type"`
	}](t, rec); body.Type != urnRateLimited {
		t.Fatalf("type = %q, want %q", body.Type, urnRateLimited)
	}

	if rec := h.loginFrom("203.0.113.8:4444", h.user.Email, h.password); rec.Code != http.StatusOK {
		t.Fatalf("status for another IP = %d, want 200: %s", rec.Code, rec.Body)
	}

	h.clock.now = h.clock.now.Add(loginWindow)
	if rec := h.loginFrom(ip, h.user.Email, h.password); rec.Code != http.StatusOK {
		t.Fatalf("status after the window = %d, want 200: %s", rec.Code, rec.Body)
	}
}

func TestMeReportsMustChangePassword(t *testing.T) {
	h := newHarness(t)
	const password = "initial password"
	hash, err := auth.HashPassword(password)
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

	// The SPA routes on this field, so login and /me must both carry it.
	if me := decode[meResponse](t, h.login(u.Email, password)); !me.MustChangePassword {
		t.Fatal("login must_change_password = false, want true")
	}
	c := sessionOf(t, h.login(u.Email, password))
	if me := decode[meResponse](t, h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value)); !me.MustChangePassword {
		t.Fatal("/me must_change_password = false, want true")
	}
}

func TestDeactivatedUserSessionIsRejected(t *testing.T) {
	h := newHarness(t)
	c := sessionOf(t, h.login(h.user.Email, h.password))
	if err := h.st.SetUserActive(h.user.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	rec := h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status with a live session after deactivation = %d, want 401: %s", rec.Code, rec.Body)
	}
}

func TestStoreFailureIsInternalError(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		h := newHarness(t)
		if err := h.st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
		rec := h.login(h.user.Email, h.password)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (a broken database is not a wrong password): %s", rec.Code, rec.Body)
		}
		if body := decode[struct {
			Type string `json:"type"`
		}](t, rec); body.Type != urnInternal {
			t.Fatalf("type = %q, want %q", body.Type, urnInternal)
		}
	})

	t.Run("me", func(t *testing.T) {
		h := newHarness(t)
		c := sessionOf(t, h.login(h.user.Email, h.password))
		if err := h.st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
		rec := h.do(http.MethodGet, "/api/v1/me", nil, nil, c.Value)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (a broken database is not an anonymous request): %s", rec.Code, rec.Body)
		}
		if body := decode[struct {
			Type string `json:"type"`
		}](t, rec); body.Type != urnInternal {
			t.Fatalf("type = %q, want %q", body.Type, urnInternal)
		}
	})
}

func TestSpecListsAuthRoutes(t *testing.T) {
	rec := get(t, "/api/v1/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []struct {
		method string
		path   string
	}{
		{"post", "/api/v1/auth/login"},
		{"post", "/api/v1/auth/logout"},
		{"get", "/api/v1/me"},
	}
	for _, w := range want {
		if doc.Paths[w.path][w.method] == nil {
			t.Errorf("spec is missing %s %s", strings.ToUpper(w.method), w.path)
		}
	}
}
