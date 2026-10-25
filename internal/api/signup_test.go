package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/clock"
)

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

type fakeSignup struct {
	startErr    error
	verifyEmail string
	verifyErr   error
	completeRes SignupResult
	completeErr error

	email string
	req   SignupRequest
}

func (f *fakeSignup) Start(_ context.Context, email string) error {
	f.email = email
	return f.startErr
}

func (f *fakeSignup) Verify(_ context.Context, _ string) (string, error) {
	return f.verifyEmail, f.verifyErr
}

func (f *fakeSignup) Complete(_ context.Context, in SignupRequest) (SignupResult, error) {
	f.req = in
	return f.completeRes, f.completeErr
}

func signupRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Origin", "http://example.com")
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "192.0.2.10:1234"
	return r
}

func serveSignup(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestSignupDisabled(t *testing.T) {
	h := NewSignupHandler(nil, clock.System{})
	for _, r := range []*http.Request{
		signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"a@b.example"}`),
		signupRequest(http.MethodGet, "/api/v1/signup/verify?token=x", ""),
		signupRequest(http.MethodPost, "/api/v1/signup/verify", `{"token":"x"}`),
	} {
		rec := serveSignup(h, r)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), urnUnavailable) {
			t.Errorf("%s %s = %d %s, want 503 unavailable", r.Method, r.URL.Path, rec.Code, rec.Body)
		}
	}
}

func TestSignupStart(t *testing.T) {
	svc := &fakeSignup{}
	h := NewSignupHandler(svc, clock.System{})

	rec := serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"ada@example.com"}`))
	if rec.Code != http.StatusAccepted || svc.email != "ada@example.com" {
		t.Fatalf("start = %d %s, email %q", rec.Code, rec.Body, svc.email)
	}

	if rec := serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{`)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad body = %d, want 422", rec.Code)
	}

	svc.startErr = &SignupInputError{Detail: "enter a valid email address"}
	rec = serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"nope"}`))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "enter a valid email address") {
		t.Fatalf("input error = %d %s, want 422 with the detail", rec.Code, rec.Body)
	}

	svc.startErr = ErrMailFailed
	rec = serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"ada@example.com"}`))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), urnEmailFailed) {
		t.Fatalf("mail failure = %d %s, want 502", rec.Code, rec.Body)
	}
}

func TestSignupRateLimitAndOrigin(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	svc := &fakeSignup{}
	h := NewSignupHandler(svc, fixedClock{at: now})

	for i := 0; i < signupLimit; i++ {
		if rec := serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"ada@example.com"}`)); rec.Code != http.StatusAccepted {
			t.Fatalf("attempt %d = %d, want 202", i+1, rec.Code)
		}
	}
	rec := serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"ada@example.com"}`))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), urnRateLimited) {
		t.Fatalf("over the limit = %d %s, want 429", rec.Code, rec.Body)
	}

	other := NewSignupHandler(&fakeSignup{}, fixedClock{at: now})
	r := signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"ada@example.com"}`)
	r.Header.Set("Origin", "https://evil.example")
	if rec := serveSignup(other, r); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin = %d, want 403", rec.Code)
	}
}

func TestSignupVerify(t *testing.T) {
	svc := &fakeSignup{verifyEmail: "ada@example.com"}
	h := NewSignupHandler(svc, clock.System{})

	rec := serveSignup(h, signupRequest(http.MethodGet, "/api/v1/signup/verify?token=t", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ada@example.com") {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body)
	}

	svc.verifyErr = ErrInvalidToken
	rec = serveSignup(h, signupRequest(http.MethodGet, "/api/v1/signup/verify?token=used", ""))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), urnInvalidToken) {
		t.Fatalf("used token = %d %s, want 404 invalid-token", rec.Code, rec.Body)
	}

	if rec := serveSignup(h, signupRequest(http.MethodDelete, "/api/v1/signup", "")); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("delete = %d, want 405", rec.Code)
	}
	if rec := serveSignup(h, signupRequest(http.MethodGet, "/api/v1/signup/nope", "")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown subpath = %d, want 404", rec.Code)
	}
}

func TestSignupComplete(t *testing.T) {
	expires := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	svc := &fakeSignup{completeRes: SignupResult{Slug: "acme", Token: "session", Expires: expires}}
	h := NewSignupHandler(svc, clock.System{})

	body := `{"token":"t","slug":"acme","name":"Ada","password":"correct horse"}`
	rec := serveSignup(h, signupRequest(http.MethodPost, "/api/v1/signup/verify", body))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"workspace":"acme"`) {
		t.Fatalf("complete = %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Header().Values("Set-Cookie")
	if len(cookies) != 1 || !strings.Contains(cookies[0], "tstamps_session=session") {
		t.Fatalf("cookies = %v, want the session cookie", cookies)
	}
	if svc.req.Slug != "acme" || svc.req.Token != "t" {
		t.Fatalf("service request = %+v", svc.req)
	}

	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{"slug taken", ErrSlugTaken, http.StatusUnprocessableEntity},
		{"used link", ErrInvalidToken, http.StatusNotFound},
		{"disabled", ErrSignupDisabled, http.StatusServiceUnavailable},
		{"input", &SignupInputError{Detail: "choose a password of at least 8 characters"}, http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeSignup{completeErr: tt.err}
			rec := serveSignup(NewSignupHandler(svc, clock.System{}),
				signupRequest(http.MethodPost, "/api/v1/signup/verify", body))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestSignupRejectsOversizedBody(t *testing.T) {
	h := NewSignupHandler(&fakeSignup{}, clock.System{})
	r := signupRequest(http.MethodPost, "/api/v1/signup", `{"email":"`+strings.Repeat("a", 5000)+`"}`)
	rec := serveSignup(h, r)
	if rec.Code != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("oversized = %d %s, want 422", rec.Code, body)
	}
}
