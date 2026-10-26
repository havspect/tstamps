package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// Mailer sends the verification message. Resend implements it in production;
// tests use a fake, so go test needs no network.
type Mailer interface {
	Send(ctx context.Context, to, subject, text string) error
}

// Signup turns a verified email into a workspace. It is the workspace side of
// api.SignupService.
type Signup struct {
	db        string
	publicURL string
	mail      Mailer
	clock     clock.Clock
}

// NewSignup builds the signup service for the shared database at db. publicURL
// is the origin the emailed link points at.
func NewSignup(db, publicURL string, mail Mailer, clk clock.Clock) *Signup {
	return &Signup{db: db, publicURL: strings.TrimSuffix(publicURL, "/"), mail: mail, clock: clk}
}

// Start stores the email behind a fresh token and emails the link. The token
// itself is never written down; only its hash names the pending row.
func (s *Signup) Start(ctx context.Context, raw string) error {
	email, err := normalizeEmail(raw)
	if err != nil {
		return err
	}
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	reg, err := store.OpenRegistry(s.db)
	if err != nil {
		return err
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		return err
	}
	if err := reg.CreatePendingSignup(hash, email, s.clock.Now()); err != nil {
		return err
	}
	link := s.publicURL + "/signup/verify?token=" + url.QueryEscape(token)
	text := "Use this link to create your Tstamps workspace:\n\n" + link +
		"\n\nThe link works once. If you did not ask for this, ignore this email.\n"
	if err := s.mail.Send(ctx, email, "Verify your email for Tstamps", text); err != nil {
		// No link reached the person, so the record would only be litter.
		_ = reg.DeletePendingSignup(hash)
		return fmt.Errorf("%w: %v", api.ErrMailFailed, err)
	}
	return nil
}

// Verify reports the email a token was sent to without using it up, so the page
// can show it before the workspace form is submitted.
func (s *Signup) Verify(_ context.Context, token string) (string, error) {
	if token == "" {
		return "", api.ErrInvalidToken
	}
	reg, err := store.OpenRegistry(s.db)
	if err != nil {
		return "", err
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		return "", err
	}
	email, err := reg.PendingSignupEmail(auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return "", api.ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	return email, nil
}

// Complete creates the workspace, its admin with the chosen password, and a
// session for that admin. The token is claimed by deleting its row, so two
// concurrent submits can never both create a workspace from one link.
func (s *Signup) Complete(ctx context.Context, in api.SignupRequest) (api.SignupResult, error) {
	slug := strings.TrimSpace(in.Slug)
	if err := ValidateSlug(slug); err != nil {
		return api.SignupResult{}, &api.SignupInputError{
			Detail: "a workspace URL may use 1 to 63 lowercase letters, digits, dots and hyphens",
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return api.SignupResult{}, &api.SignupInputError{Detail: "enter your name"}
	}
	if len(in.Password) < 8 {
		return api.SignupResult{}, &api.SignupInputError{Detail: "choose a password of at least 8 characters"}
	}
	if in.Token == "" {
		return api.SignupResult{}, api.ErrInvalidToken
	}
	hash := auth.HashToken(in.Token)
	reg, err := store.OpenRegistry(s.db)
	if err != nil {
		return api.SignupResult{}, err
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		return api.SignupResult{}, err
	}
	email, err := reg.ClaimPendingSignup(hash)
	if errors.Is(err, store.ErrNotFound) {
		return api.SignupResult{}, api.ErrInvalidToken
	}
	if err != nil {
		return api.SignupResult{}, err
	}

	if err := CreateWithPassword(s.db, slug, email, name, in.Password); err != nil {
		// The form can be corrected with the same link, so put the claim back.
		_ = reg.CreatePendingSignup(hash, email, s.clock.Now())
		if errors.Is(err, ErrExists) {
			return api.SignupResult{}, api.ErrSlugTaken
		}
		return api.SignupResult{}, err
	}

	// CreateWithPassword committed the row, so the store can now find it by slug.
	w, err := reg.WorkspaceBySlug(slug)
	if err != nil {
		return api.SignupResult{}, err
	}
	st := reg.ForWorkspace(w.ID)
	u, err := st.UserByEmail(email)
	if err != nil {
		return api.SignupResult{}, err
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return api.SignupResult{}, err
	}
	expires := s.clock.Now().Add(auth.SessionTTL)
	if err := st.CreateSession(&store.Session{UserID: u.ID, TokenHash: tokenHash, ExpiresAt: expires}); err != nil {
		return api.SignupResult{}, err
	}
	return api.SignupResult{Slug: slug, Token: token, Expires: expires}, nil
}

// normalizeEmail lowercases and validates the address the link is sent to.
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", &api.SignupInputError{Detail: "enter a valid email address"}
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", &api.SignupInputError{Detail: "enter a valid email address"}
	}
	return email, nil
}
