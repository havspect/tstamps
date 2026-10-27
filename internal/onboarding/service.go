package onboarding

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/havspect/tstamps/internal/auth"
)

// Common onboarding errors.
var (
	ErrSignupDisabled   = errors.New("self-service signup is not enabled on this server")
	ErrInvalidSlug      = errors.New("invalid workspace slug")
	ErrSlugTaken        = errors.New("workspace slug already taken")
	ErrNotFound         = errors.New("not found")
	ErrInvalidToken     = errors.New("invalid or expired signup token")
	ErrInvalidEmail     = errors.New("invalid email address")
	ErrInvalidName      = errors.New("name is required")
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)

// ValidateSlug accepts lowercase letters, digits, dots, and hyphens (1-63 chars).
func ValidateSlug(slug string) error {
	if len(slug) == 0 || len(slug) > 63 {
		return ErrInvalidSlug
	}
	if slug[0] == '.' || slug[len(slug)-1] == '.' || strings.Contains(slug, "..") {
		return ErrInvalidSlug
	}
	for i, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '.' || r == '-') && i > 0 && i < len(slug)-1:
		default:
			return ErrInvalidSlug
		}
	}
	return nil
}

// Service coordinates onboarding workflows.
type Service struct {
	repo      Repository
	mailer    Mailer
	publicURL string
}

// NewService constructs an onboarding Service with the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// WithMailer configures email sending for self-service signup.
func (s *Service) WithMailer(m Mailer, publicURL string) *Service {
	s.mailer = m
	s.publicURL = strings.TrimSuffix(publicURL, "/")
	return s
}

// ValidateSlug validates workspace slug syntax.
func (s *Service) ValidateSlug(slug string) error {
	return ValidateSlug(slug)
}

// CreateWorkspace creates a workspace and its initial admin user with a generated password.
// Returns the workspace domain object and the plain-text initial password.
func (s *Service) CreateWorkspace(ctx context.Context, slug, email, name string) (*Workspace, string, error) {
	slug = strings.TrimSpace(slug)
	if err := s.ValidateSlug(slug); err != nil {
		return nil, "", err
	}

	normEmail, err := normalizeEmail(email)
	if err != nil {
		return nil, "", err
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, "", ErrInvalidName
	}

	pwd, err := auth.GeneratePassword()
	if err != nil {
		return nil, "", err
	}

	pwdHash, err := auth.HashPassword(pwd)
	if err != nil {
		return nil, "", err
	}

	ws, err := s.repo.CreateWorkspace(ctx, slug)
	if err != nil {
		return nil, "", err
	}

	_, err = s.repo.CreateInitialAdmin(ctx, ws.ID, normEmail, trimmedName, pwdHash, "UTC")
	if err != nil {
		return nil, "", err
	}

	return ws, pwd, nil
}

// StartSignup initiates email verification for a new workspace signup.
// It generates a secure signup token, stores its hash, and returns the raw token.
func (s *Service) StartSignup(ctx context.Context, email string) (string, error) {
	if s.mailer == nil {
		return "", ErrSignupDisabled
	}

	normEmail, err := normalizeEmail(email)
	if err != nil {
		return "", err
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return "", err
	}

	if err := s.repo.CreatePendingSignup(ctx, normEmail, hash); err != nil {
		return "", err
	}

	if s.mailer != nil {
		link := s.publicURL + "/signup/verify?token=" + url.QueryEscape(token)
		subject := "Confirm your Tstamps workspace"
		text := "Open this link to set up your workspace and choose a password:\n\n" + link + "\n\nIt expires in 24 hours. If you did not ask for this, ignore this email."
		if err := s.mailer.Send(ctx, normEmail, subject, text); err != nil {
			return "", fmt.Errorf("send email: %w", err)
		}
	}

	return token, nil
}

// VerifySignup checks whether a signup token is valid and returns the associated email.
func (s *Service) VerifySignup(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrInvalidToken
	}

	hash := auth.HashToken(token)
	pending, err := s.repo.GetPendingSignup(ctx, hash)
	if err != nil {
		return "", ErrInvalidToken
	}

	return pending.Email, nil
}

// CompleteSignup finalizes signup: consumes the token, creates the workspace and admin,
// and returns a SignupResult with an initial session token.
func (s *Service) CompleteSignup(ctx context.Context, token, slug, name, password string) (*SignupResult, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidToken
	}

	slug = strings.TrimSpace(slug)
	if err := s.ValidateSlug(slug); err != nil {
		return nil, err
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, ErrInvalidName
	}

	if len(password) < 8 {
		return nil, ErrPasswordTooShort
	}

	hash := auth.HashToken(token)
	pending, err := s.repo.GetPendingSignup(ctx, hash)
	if err != nil {
		return nil, ErrInvalidToken
	}

	pwdHash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	ws, err := s.repo.CreateWorkspace(ctx, slug)
	if err != nil {
		return nil, err
	}

	adminID, err := s.repo.CreateInitialAdmin(ctx, ws.ID, pending.Email, trimmedName, pwdHash, "UTC")
	if err != nil {
		return nil, err
	}

	_ = s.repo.DeletePendingSignup(ctx, hash)

	sessionToken, _, err := auth.NewSessionToken()
	if err != nil {
		return nil, err
	}

	return &SignupResult{
		WorkspaceSlug: ws.Slug,
		Email:         pending.Email,
		Name:          trimmedName,
		Token:         sessionToken,
		UserID:        adminID,
	}, nil
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}
