package client

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/onboarding"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

const testPassword = "correct horse battery staple"

// newServer starts the real API over a store with one active user.
func newServer(t *testing.T, email string) *httptest.Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tstamps.db")
	handler, db := buildTestServer(t, dbPath)

	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	onbRepo := gormrepo.NewOnboardingRepository(db)
	ws, err := onbRepo.CreateWorkspace(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	_, err = onbRepo.CreateInitialAdmin(context.Background(), ws.ID, email, "Ada Lovelace", hash, "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestLogin(t *testing.T) {
	srv := newServer(t, "ada@example.com")
	c := New(Config{URL: srv.URL})

	me, err := c.Login("ada@example.com", testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if me.Email != "ada@example.com" || me.Name != "Ada Lovelace" || me.ID == 0 {
		t.Fatalf("me = %+v, want the seeded user", me)
	}

	_, err = c.Login("ada@example.com", "wrong")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Fatalf("wrong password error = %v, want a 401 *Error", err)
	}
	if !strings.Contains(apiErr.Error(), "incorrect") {
		t.Fatalf("error %q does not carry the server detail", apiErr)
	}
}

func TestWorkspaceBaseURL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tstamps.db")
	handler, db := buildTestServer(t, dbPath)

	onbRepo := gormrepo.NewOnboardingRepository(db)
	onbSvc := onboarding.NewService(onbRepo)
	_, password, err := onbSvc.CreateWorkspace(context.Background(), "example.com", "ada@example.com", "Ada Lovelace")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := New(Config{URL: srv.URL + "/w/example.com/"})
	me, err := c.Login("ada@example.com", password)
	if err != nil {
		t.Fatalf("login through workspace URL: %v", err)
	}
	if me.Email != "ada@example.com" {
		t.Fatalf("me = %+v", me)
	}
	if _, err := c.Me(); err != nil {
		t.Fatalf("me through saved cookie: %v", err)
	}
}

func TestCreateTokenThenBearer(t *testing.T) {
	srv := newServer(t, "ada@example.com")
	c := New(Config{URL: srv.URL})
	if _, err := c.Login("ada@example.com", testPassword); err != nil {
		t.Fatalf("login: %v", err)
	}

	tok, err := c.CreateToken("laptop")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok.ID == 0 || tok.Name != "laptop" || tok.Token == "" {
		t.Fatalf("created token = %+v, want an id, name and raw value", tok)
	}
	if !strings.HasPrefix(tok.Token, "ts_") {
		t.Fatalf("token %q does not start with ts_", tok.Token)
	}

	bearer := New(Config{URL: srv.URL, Token: tok.Token})
	me, err := bearer.Me()
	if err != nil {
		t.Fatalf("me with bearer: %v", err)
	}
	if me.Email != "ada@example.com" {
		t.Fatalf("bearer me email = %q, want ada@example.com", me.Email)
	}

	_, err = New(Config{URL: srv.URL, Token: "ts_bogus"}).Me()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Fatalf("bogus token error = %v, want a 401 *Error", err)
	}
}
