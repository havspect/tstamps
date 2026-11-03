package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/onboarding"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

type signupMailer struct {
	to   string
	text string
}

func (m *signupMailer) Send(_ context.Context, to, _, text string) error {
	m.to = to
	m.text = text
	return nil
}

// TestSignupDoesNotReturnTokens proves the signup flow hands the session to the
// browser only through the session cookie.
func TestSignupDoesNotReturnTokens(t *testing.T) {
	handler, deps := setupTestServer(t)
	mailer := &signupMailer{}
	deps.OnboardingService = onboarding.NewService(gormrepo.NewOnboardingRepository(deps.DB)).
		WithMailer(mailer, "http://example.com")
	handler = New(*deps)

	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://example.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := post("/api/v1/signup", `{"email":"ada@example.com"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start signup = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	var start map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &start); err != nil {
		t.Fatalf("decode start signup: %v", err)
	}
	if _, ok := start["token"]; ok {
		t.Errorf("start signup returned a token: %s", rec.Body.String())
	}
	if mailer.to != "ada@example.com" {
		t.Fatalf("mail to = %q, want ada@example.com", mailer.to)
	}

	var token string
	for _, field := range strings.Fields(mailer.text) {
		u, err := url.Parse(field)
		if err != nil || u.Path != "/signup/verify" {
			continue
		}
		token = u.Query().Get("token")
	}
	if token == "" {
		t.Fatalf("no signup link in mail: %q", mailer.text)
	}

	rec = post("/api/v1/signup/verify",
		`{"token":"`+token+`","slug":"acme","name":"Ada Lovelace","password":"correct horse"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("complete signup = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var complete map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &complete); err != nil {
		t.Fatalf("decode complete signup: %v", err)
	}
	if _, ok := complete["token"]; ok {
		t.Errorf("complete signup returned a token: %s", rec.Body.String())
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatalf("no %s cookie in %v", sessionCookie, rec.Result().Cookies())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(session)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/me with signup cookie = %d, want 200: %s", meRec.Code, meRec.Body.String())
	}
	var me struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.Email != "ada@example.com" {
		t.Errorf("me email = %q, want ada@example.com", me.Email)
	}
}
