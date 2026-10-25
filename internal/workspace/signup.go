package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/havspect/tstamps/internal/api"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// pendingDir holds one file per emailed signup until its link is used. It is a
// directory on purpose: Databases and backup only look at *.db files, so a
// pending signup is never mistaken for a workspace or included in a backup.
const pendingDir = "pending"

// Mailer sends the verification message. Resend implements it in production;
// tests use a fake, so go test needs no network.
type Mailer interface {
	Send(ctx context.Context, to, subject, text string) error
}

// Signup turns a verified email into a workspace. It is the workspace side of
// api.SignupService.
type Signup struct {
	dir       string
	publicURL string
	mail      Mailer
	clock     clock.Clock
}

// NewSignup builds the signup service for the workspace directory. publicURL is
// the origin the emailed link points at.
func NewSignup(dir, publicURL string, mail Mailer, clk clock.Clock) *Signup {
	return &Signup{dir: dir, publicURL: strings.TrimSuffix(publicURL, "/"), mail: mail, clock: clk}
}

// Start stores the email behind a fresh token and emails the link. The token
// itself is never written down; only its hash names the pending file.
func (s *Signup) Start(ctx context.Context, raw string) error {
	email, err := normalizeEmail(raw)
	if err != nil {
		return err
	}
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	if err := s.writePending(hash, email); err != nil {
		return err
	}
	link := s.publicURL + "/signup/verify?token=" + url.QueryEscape(token)
	text := "Use this link to create your Tstamps workspace:\n\n" + link +
		"\n\nThe link works once. If you did not ask for this, ignore this email.\n"
	if err := s.mail.Send(ctx, email, "Verify your email for Tstamps", text); err != nil {
		// No link reached the person, so the record would only be litter.
		_ = s.removePending(hash)
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
	email, err := s.readPending(auth.HashToken(token))
	if errors.Is(err, fs.ErrNotExist) {
		return "", api.ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	return email, nil
}

// Complete creates the workspace, its admin with the chosen password, and a
// session for that admin. The token is claimed by an atomic rename, so two
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
	email, err := s.claim(hash)
	if err != nil {
		return api.SignupResult{}, err
	}

	if err := CreateWithPassword(s.dir, slug, email, name, in.Password); err != nil {
		// The form can be corrected with the same link, so put the claim back.
		s.release(hash)
		if errors.Is(err, ErrExists) {
			return api.SignupResult{}, api.ErrSlugTaken
		}
		return api.SignupResult{}, err
	}
	s.discard(hash)

	db, err := DatabasePath(s.dir, slug)
	if err != nil {
		return api.SignupResult{}, err
	}
	st, err := store.Open(db)
	if err != nil {
		return api.SignupResult{}, err
	}
	defer st.Close()
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

// writePending keeps only the address; the capability is the token, whose hash
// is the file name. shortcut: pending signups never expire (no TTL by design), so
// a link nobody clicks stays on disk until someone removes the directory.
func (s *Signup) writePending(hash, email string) error {
	dir := filepath.Join(s.dir, pendingDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Email string `json:"email"`
	}{email})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, hash+".json"), b, 0o600)
}

func (s *Signup) pendingPath(hash string) string {
	return filepath.Join(s.dir, pendingDir, hash+".json")
}

// claim renames the pending record to its used name. Rename is atomic, so exactly
// one submit can claim a link even when two arrive together.
func (s *Signup) claim(hash string) (string, error) {
	used := s.pendingPath(hash) + ".used"
	if err := os.Rename(s.pendingPath(hash), used); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", api.ErrInvalidToken
		}
		return "", err
	}
	b, err := os.ReadFile(used)
	if err != nil {
		s.release(hash)
		return "", err
	}
	var rec struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		s.release(hash)
		return "", err
	}
	return rec.Email, nil
}

// release puts a claim back, so a form error does not burn the link.
func (s *Signup) release(hash string) {
	_ = os.Rename(s.pendingPath(hash)+".used", s.pendingPath(hash))
}

func (s *Signup) discard(hash string) {
	_ = os.Remove(s.pendingPath(hash) + ".used")
}

func (s *Signup) readPending(hash string) (string, error) {
	b, err := os.ReadFile(s.pendingPath(hash))
	if err != nil {
		return "", err
	}
	var rec struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", err
	}
	return rec.Email, nil
}

func (s *Signup) removePending(hash string) error {
	err := os.Remove(s.pendingPath(hash))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
