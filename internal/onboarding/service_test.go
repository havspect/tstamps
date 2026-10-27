package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryRepo struct {
	workspaces     map[uint]*Workspace
	workspaceSlugs map[string]*Workspace
	pendingSignups map[string]*PendingSignup
	admins         map[uint]string
	nextID         uint
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		workspaces:     make(map[uint]*Workspace),
		workspaceSlugs: make(map[string]*Workspace),
		pendingSignups: make(map[string]*PendingSignup),
		admins:         make(map[uint]string),
		nextID:         1,
	}
}

func (m *memoryRepo) CreateWorkspace(_ context.Context, slug string) (*Workspace, error) {
	if _, exists := m.workspaceSlugs[slug]; exists {
		return nil, ErrSlugTaken
	}
	ws := &Workspace{
		ID:        m.nextID,
		Slug:      slug,
		CreatedAt: time.Now(),
	}
	m.nextID++
	m.workspaces[ws.ID] = ws
	m.workspaceSlugs[slug] = ws
	return ws, nil
}

func (m *memoryRepo) GetWorkspaceBySlug(_ context.Context, slug string) (*Workspace, error) {
	ws, ok := m.workspaceSlugs[slug]
	if !ok {
		return nil, ErrNotFound
	}
	return ws, nil
}

func (m *memoryRepo) ListWorkspaces(_ context.Context) ([]Workspace, error) {
	res := make([]Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		res = append(res, *ws)
	}
	return res, nil
}

func (m *memoryRepo) CreatePendingSignup(_ context.Context, email, tokenHash string) error {
	m.pendingSignups[tokenHash] = &PendingSignup{
		TokenHash: tokenHash,
		Email:     email,
		CreatedAt: time.Now(),
	}
	return nil
}

func (m *memoryRepo) GetPendingSignup(_ context.Context, tokenHash string) (*PendingSignup, error) {
	p, ok := m.pendingSignups[tokenHash]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (m *memoryRepo) DeletePendingSignup(_ context.Context, tokenHash string) error {
	delete(m.pendingSignups, tokenHash)
	return nil
}

func (m *memoryRepo) CreateInitialAdmin(_ context.Context, workspaceID uint, email, name, passwordHash, timezone string) (uint, error) {
	adminID := m.nextID
	m.nextID++
	m.admins[adminID] = email
	return adminID, nil
}

func TestValidateSlug(t *testing.T) {
	valid := []string{"acme", "my-workspace", "team.alpha", "sub-domain.org", "w123"}
	for _, slug := range valid {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("expected valid slug %q, got: %v", slug, err)
		}
	}

	invalid := []string{
		"",
		"UPPERCASE",
		"-leadingslash",
		".leadingdot",
		"trailingdot.",
		"two..dots",
		"space in slug",
		"slash/slug",
		"verylongslugverylongslugverylongslugverylongslugverylongslugverylongslug",
	}
	for _, slug := range invalid {
		if err := ValidateSlug(slug); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("expected ErrInvalidSlug for %q, got: %v", slug, err)
		}
	}
}

func TestService_CreateWorkspace(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)
	ctx := context.Background()

	// 1. Successful workspace creation
	ws, pwd, err := svc.CreateWorkspace(ctx, "acme", "admin@acme.com", "Alice Admin")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if ws.Slug != "acme" {
		t.Errorf("expected slug 'acme', got %q", ws.Slug)
	}
	if len(pwd) == 0 {
		t.Errorf("expected non-empty generated password")
	}

	// 2. Duplicate workspace
	_, _, err = svc.CreateWorkspace(ctx, "acme", "admin2@acme.com", "Bob Admin")
	if !errors.Is(err, ErrSlugTaken) {
		t.Errorf("expected ErrSlugTaken for duplicate slug, got %v", err)
	}

	// 3. Invalid slug
	_, _, err = svc.CreateWorkspace(ctx, "bad slug!", "admin@acme.com", "Admin")
	if !errors.Is(err, ErrInvalidSlug) {
		t.Errorf("expected ErrInvalidSlug, got %v", err)
	}

	// 4. Invalid email
	_, _, err = svc.CreateWorkspace(ctx, "newcorp", "notanemail", "Admin")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail, got %v", err)
	}

	// 5. Invalid name
	_, _, err = svc.CreateWorkspace(ctx, "newcorp", "admin@newcorp.com", "")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

type fakeMailer struct{}

func (f *fakeMailer) Send(_ context.Context, _, _, _ string) error {
	return nil
}

func TestService_SignupFlow(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)
	ctx := context.Background()

	// 0. Signup disabled without mailer
	if _, err := svc.StartSignup(ctx, "signup@example.com"); !errors.Is(err, ErrSignupDisabled) {
		t.Fatalf("expected ErrSignupDisabled, got %v", err)
	}

	svc.WithMailer(&fakeMailer{}, "http://localhost:8080")

	// 1. StartSignup
	token, err := svc.StartSignup(ctx, "signup@example.com")
	if err != nil {
		t.Fatalf("StartSignup: %v", err)
	}
	if len(token) == 0 {
		t.Fatalf("expected non-empty token")
	}

	// Invalid email
	if _, err := svc.StartSignup(ctx, "invalid-email"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("expected ErrInvalidEmail, got %v", err)
	}

	// 2. VerifySignup
	email, err := svc.VerifySignup(ctx, token)
	if err != nil {
		t.Fatalf("VerifySignup: %v", err)
	}
	if email != "signup@example.com" {
		t.Errorf("expected 'signup@example.com', got %q", email)
	}

	// Verify with unknown or invalid token
	if _, err := svc.VerifySignup(ctx, "nonexistent-token"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for invalid token, got %v", err)
	}
	if _, err := svc.VerifySignup(ctx, ""); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for empty token, got %v", err)
	}

	// 3. CompleteSignup validation errors
	// Bad token
	_, err = svc.CompleteSignup(ctx, "unknown-token", "myws", "Founder", "secretpassword123")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	// Bad slug
	_, err = svc.CompleteSignup(ctx, token, "Bad Slug", "Founder", "secretpassword123")
	if !errors.Is(err, ErrInvalidSlug) {
		t.Errorf("expected ErrInvalidSlug, got %v", err)
	}
	// Bad name
	_, err = svc.CompleteSignup(ctx, token, "myws", "", "secretpassword123")
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
	// Short password
	_, err = svc.CompleteSignup(ctx, token, "myws", "Founder", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("expected ErrPasswordTooShort, got %v", err)
	}

	// 4. CompleteSignup success
	res, err := svc.CompleteSignup(ctx, token, "myws", "Founder", "secretpassword123")
	if err != nil {
		t.Fatalf("CompleteSignup: %v", err)
	}
	if res.WorkspaceSlug != "myws" {
		t.Errorf("expected slug 'myws', got %q", res.WorkspaceSlug)
	}
	if res.Email != "signup@example.com" {
		t.Errorf("expected email 'signup@example.com', got %q", res.Email)
	}
	if res.Name != "Founder" {
		t.Errorf("expected name 'Founder', got %q", res.Name)
	}
	if len(res.Token) == 0 {
		t.Errorf("expected non-empty session token")
	}
	if res.UserID == 0 {
		t.Errorf("expected positive user ID")
	}

	// 5. Reusing the token should fail because pending signup was consumed
	_, err = svc.CompleteSignup(ctx, token, "myws2", "Founder", "secretpassword123")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken on consumed token replay, got %v", err)
	}
}
