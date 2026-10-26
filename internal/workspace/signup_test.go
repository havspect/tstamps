package workspace

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/store"
)

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

type fakeMailer struct {
	err   error
	sent  int
	to    string
	text  string
	links []string
}

func (m *fakeMailer) Send(_ context.Context, to, _, text string) error {
	m.sent++
	m.to, m.text = to, text
	for _, field := range strings.Fields(text) {
		if strings.Contains(field, "/signup/verify?token=") {
			m.links = append(m.links, field)
		}
	}
	return m.err
}

func linkToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link %q: %v", link, err)
	}
	token := u.Query().Get("token")
	if token == "" {
		t.Fatalf("link %q has no token", link)
	}
	return token
}

func TestSignupCreatesWorkspaceAfterVerification(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	mail := &fakeMailer{}
	s := NewSignup(db, "https://time.example.com/", mail, fixedClock{at: now})
	ctx := context.Background()

	if err := s.Start(ctx, "  Ada@Example.COM "); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if mail.sent != 1 || mail.to != "ada@example.com" {
		t.Fatalf("mail = %+v, want one message to the lowercased address", mail)
	}
	if len(mail.links) != 1 || !strings.HasPrefix(mail.links[0], "https://time.example.com/signup/verify?token=") {
		t.Fatalf("links = %v, want the public signup URL", mail.links)
	}
	token := linkToken(t, mail.links[0])

	// No workspace exists until the link is used, but the pending row does.
	reg, err := store.OpenRegistry(db)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer reg.Close()
	workspaces, err := reg.Workspaces()
	if err != nil || len(workspaces) != 0 {
		t.Fatalf("workspaces before verification = %v, %v; want none", workspaces, err)
	}
	if _, err := reg.PendingSignupEmail(auth.HashToken(token)); err != nil {
		t.Fatalf("pending record missing: %v", err)
	}

	email, err := s.Verify(ctx, token)
	if err != nil || email != "ada@example.com" {
		t.Fatalf("Verify = %q, %v", email, err)
	}

	res, err := s.Complete(ctx, api.SignupRequest{
		Token: token, Slug: "acme", Name: "Ada Lovelace", Password: "correct horse",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Slug != "acme" || res.Token == "" {
		t.Fatalf("result = %+v, want a slug and a session token", res)
	}
	if !res.Expires.Equal(now.Add(auth.SessionTTL)) {
		t.Fatalf("expiry = %v, want the clock plus the session TTL", res.Expires)
	}

	st, err := openWorkspace(t, db, "acme")
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	u, err := st.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("admin missing: %v", err)
	}
	if !u.IsAdmin || !u.Active || u.MustChangePassword || u.Name != "Ada Lovelace" {
		t.Fatalf("admin = %+v, want an active admin who chose the password", u)
	}
	if !auth.VerifyPassword(u.PasswordHash, "correct horse") {
		t.Fatal("the chosen password does not verify")
	}
	if _, err := st.SessionByTokenHash(auth.HashToken(res.Token)); err != nil {
		t.Fatalf("signup session missing: %v", err)
	}

	// The link is single use, and its pending row is gone.
	if _, err := s.Verify(ctx, token); !errors.Is(err, api.ErrInvalidToken) {
		t.Fatalf("second Verify = %v, want ErrInvalidToken", err)
	}
	if _, err := s.Complete(ctx, api.SignupRequest{
		Token: token, Slug: "other", Name: "Ada", Password: "correct horse",
	}); !errors.Is(err, api.ErrInvalidToken) {
		t.Fatalf("second Complete = %v, want ErrInvalidToken", err)
	}
	if _, err := reg.PendingSignupEmail(auth.HashToken(token)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("pending record survived: %v", err)
	}
}

func TestSignupRejectsBadInputAndKeepsTheLink(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	mail := &fakeMailer{}
	s := NewSignup(db, "https://time.example.com", mail, fixedClock{at: time.Now()})
	ctx := context.Background()

	var input *api.SignupInputError
	if err := s.Start(ctx, "not-an-email"); !errors.As(err, &input) {
		t.Fatalf("Start(invalid) = %v, want SignupInputError", err)
	}
	if mail.sent != 0 {
		t.Fatal("an invalid address must not send mail")
	}

	// A mail failure leaves no pending record behind to click.
	mail.err = errors.New("smtp down")
	if err := s.Start(ctx, "ada@example.com"); !errors.Is(err, api.ErrMailFailed) {
		t.Fatalf("Start(mail failure) = %v, want ErrMailFailed", err)
	}
	failedToken := linkToken(t, mail.links[0])
	reg, err := store.OpenRegistry(db)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	if _, err := reg.PendingSignupEmail(auth.HashToken(failedToken)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("pending record after failed send: %v", err)
	}
	_ = reg.Close()

	mail.err = nil
	if err := s.Start(ctx, "ada@example.com"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	token := linkToken(t, mail.links[1])

	for _, tt := range []struct {
		name string
		in   api.SignupRequest
	}{
		{"slug", api.SignupRequest{Token: token, Slug: "Bad Slug", Name: "Ada", Password: "correct horse"}},
		{"name", api.SignupRequest{Token: token, Slug: "acme", Name: "  ", Password: "correct horse"}},
		{"password", api.SignupRequest{Token: token, Slug: "acme", Name: "Ada", Password: "short"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Complete(ctx, tt.in); !errors.As(err, &input) {
				t.Fatalf("Complete = %v, want SignupInputError", err)
			}
		})
	}
	if _, err := s.Verify(ctx, token); err != nil {
		t.Fatalf("a rejected form consumed the link: %v", err)
	}
}

func TestSignupSlugCollisionKeepsTheLinkUsable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	if _, err := Create(db, "acme", "admin@example.com", "Admin"); err != nil {
		t.Fatal(err)
	}
	mail := &fakeMailer{}
	s := NewSignup(db, "https://time.example.com", mail, fixedClock{at: time.Now()})
	ctx := context.Background()
	if err := s.Start(ctx, "ada@example.com"); err != nil {
		t.Fatal(err)
	}
	token := linkToken(t, mail.links[0])

	if _, err := s.Complete(ctx, api.SignupRequest{
		Token: token, Slug: "acme", Name: "Ada", Password: "correct horse",
	}); !errors.Is(err, api.ErrSlugTaken) {
		t.Fatalf("Complete = %v, want ErrSlugTaken", err)
	}
	if _, err := s.Verify(ctx, token); err != nil {
		t.Fatalf("slug collision consumed the link: %v", err)
	}
	if _, err := s.Complete(ctx, api.SignupRequest{
		Token: token, Slug: "acme-two", Name: "Ada", Password: "correct horse",
	}); err != nil {
		t.Fatalf("retry with a free slug: %v", err)
	}
}

func TestSignupClaimIsSingleUseUnderConcurrency(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	mail := &fakeMailer{}
	s := NewSignup(db, "https://time.example.com", mail, fixedClock{at: time.Now()})
	ctx := context.Background()
	if err := s.Start(ctx, "ada@example.com"); err != nil {
		t.Fatal(err)
	}
	token := linkToken(t, mail.links[0])

	results := make(chan error, 2)
	for _, slug := range []string{"one", "two"} {
		go func(slug string) {
			_, err := s.Complete(ctx, api.SignupRequest{
				Token: token, Slug: slug, Name: "Ada", Password: "correct horse",
			})
			results <- err
		}(slug)
	}
	var created int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			created++
		case errors.Is(err, api.ErrInvalidToken):
		default:
			t.Fatalf("Complete = %v, want success or ErrInvalidToken", err)
		}
	}
	if created != 1 {
		t.Fatalf("created %d workspaces from one link, want 1", created)
	}
	reg, err := store.OpenRegistry(db)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	n, err := reg.Workspaces()
	if err != nil || len(n) != 1 {
		t.Fatalf("Workspaces = %v, %v; want exactly one workspace", n, err)
	}
	_ = reg.Close()
	if _, err := s.Verify(ctx, token); !errors.Is(err, api.ErrInvalidToken) {
		t.Fatalf("link still verifies after creation: %v", err)
	}
}
