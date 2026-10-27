package user

import (
	"context"
	"errors"
	"strings"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Service coordinates user lifecycle, credentials, personal tokens, and authorization invariants.
type Service struct {
	repo         Repository
	auditor      Auditor
	timerStopper TimerStopper
	clock        clock.Clock
}

// New creates a new user Service.
func New(repo Repository, auditor Auditor, stopper TimerStopper, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{
		repo:         repo,
		auditor:      auditor,
		timerStopper: stopper,
		clock:        clk,
	}
}

func requireAdmin(ctx context.Context) (*auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if !p.IsAdmin() {
		return nil, authz.ErrForbidden
	}
	return p, nil
}

func auditFields(u *User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"id":                   u.ID,
		"email":                u.Email,
		"name":                 u.Name,
		"timezone":             u.Timezone,
		"is_admin":             u.IsAdmin,
		"active":               u.Active,
		"must_change_password": u.MustChangePassword,
		"created_at":           u.CreatedAt,
		"updated_at":           u.UpdatedAt,
	}
}

// CreateUser provisions a new user with a generated password and must_change_password=true. Requires administrator role.
func (s *Service) CreateUser(ctx context.Context, in CreateInput) (*User, string, error) {
	p, err := requireAdmin(ctx)
	if err != nil {
		return nil, "", err
	}

	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return nil, "", ErrInvalidEmail
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "", ErrInvalidName
	}
	tz := strings.TrimSpace(in.Timezone)
	if tz == "" {
		tz = "UTC"
	}

	if existing, err := s.repo.GetByEmail(ctx, email); err == nil && existing != nil {
		return nil, "", ErrDuplicateEmail
	}

	password, err := auth.GeneratePassword()
	if err != nil {
		return nil, "", err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	now := s.clock.Now()
	u := &User{
		WorkspaceID:        p.WorkspaceID,
		Email:              email,
		Name:               name,
		PasswordHash:       hash,
		Timezone:           tz,
		IsAdmin:            in.IsAdmin,
		Active:             true,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, "", err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "user.create", "user", u.ID, map[string]any{
			"after": auditFields(u),
		})
	}

	return u, password, nil
}

// UpdateUser applies modifications to a user account, enforcing the last active admin invariant
// and cascading deactivation (stopping running timers and revoking OAuth tokens). Requires administrator role.
func (s *Service) UpdateUser(ctx context.Context, in UpdateInput) (*User, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}

	before, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	after := *before
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, ErrInvalidName
		}
		after.Name = name
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		after.Timezone = tz
	}
	if in.IsAdmin != nil {
		after.IsAdmin = *in.IsAdmin
	}
	if in.Active != nil {
		after.Active = *in.Active
	}

	// Enforce invariant: cannot demote or deactivate the last active admin
	if before.IsAdmin && before.Active && (!after.IsAdmin || !after.Active) {
		count, err := s.repo.CountActiveAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if count <= 1 {
			return nil, ErrLastAdmin
		}
	}

	// Handle deactivation cascades
	if before.Active && !after.Active {
		if s.timerStopper != nil {
			err := s.timerStopper.StopRunningTimer(ctx, in.ID, "stopped on deactivation")
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		if err := s.repo.RevokeAllOAuthTokens(ctx, in.ID); err != nil {
			return nil, err
		}
	}

	after.UpdatedAt = s.clock.Now()
	if err := s.repo.Update(ctx, &after); err != nil {
		return nil, err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "user.update", "user", in.ID, map[string]any{
			"before": auditFields(before),
			"after":  auditFields(&after),
		})
	}

	return &after, nil
}

// ResetPassword resets a user's password, invalidates all sessions and tokens, and marks must_change_password=true. Requires administrator role.
func (s *Service) ResetPassword(ctx context.Context, id uint) (string, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return "", err
	}

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	newPassword, err := auth.GeneratePassword()
	if err != nil {
		return "", err
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return "", err
	}

	now := s.clock.Now()
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.UpdatedAt = now

	if err := s.repo.Update(ctx, u); err != nil {
		return "", err
	}

	if err := s.repo.DeleteUserSessionsExcept(ctx, id, ""); err != nil {
		return "", err
	}

	if err := s.repo.RevokeAllOAuthTokens(ctx, id); err != nil {
		return "", err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "user.password.reset", "user", id, map[string]any{
			"before": auditFields(u),
		})
	}

	return newPassword, nil
}

// ChangeOwnPassword allows an authenticated user to change their own password upon verifying current password.
func (s *Service) ChangeOwnPassword(ctx context.Context, currentPass, newPass string) error {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return authz.ErrUnauthorized
	}

	u, err := s.repo.GetByID(ctx, p.UserID)
	if err != nil {
		return err
	}

	if !auth.VerifyPassword(u.PasswordHash, currentPass) {
		return ErrInvalidPassword
	}

	if len(newPass) < 8 {
		return ErrPasswordTooShort
	}

	hash, err := auth.HashPassword(newPass)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.UpdatedAt = now

	if err := s.repo.Update(ctx, u); err != nil {
		return err
	}

	if err := s.repo.DeleteUserSessionsExcept(ctx, u.ID, p.TokenID); err != nil {
		return err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "user.password.change", "user", u.ID, map[string]any{
			"after": auditFields(u),
		})
	}

	return nil
}

// GetProfile returns the profile of the current authenticated user along with their led team IDs.
func (s *Service) GetProfile(ctx context.Context) (*Profile, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	u, err := s.repo.GetByID(ctx, p.UserID)
	if err != nil {
		return nil, err
	}

	return &Profile{
		User:       *u,
		LedTeamIDs: p.LedTeamIDs,
	}, nil
}

// ListUsers returns all users in the workspace. Requires administrator role.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}

	return s.repo.List(ctx)
}

// CreatePersonalToken creates a new personal API token for the current user and returns the token and raw secret.
func (s *Service) CreatePersonalToken(ctx context.Context, name string) (*APIToken, string, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, "", authz.ErrUnauthorized
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", ErrInvalidTokenName
	}

	token, hash, err := auth.NewAPIToken()
	if err != nil {
		return nil, "", err
	}

	now := s.clock.Now()
	t := &APIToken{
		WorkspaceID: p.WorkspaceID,
		UserID:      p.UserID,
		Name:        name,
		TokenHash:   hash,
		CreatedAt:   now,
	}

	if err := s.repo.CreateAPIToken(ctx, t); err != nil {
		return nil, "", err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "token.create", "token", t.ID, map[string]any{
			"name": name,
		})
	}

	return t, token, nil
}

// RevokePersonalToken revokes a personal API token belonging to the current user.
func (s *Service) RevokePersonalToken(ctx context.Context, id uint) error {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return authz.ErrUnauthorized
	}

	if err := s.repo.RevokeAPIToken(ctx, id, p.UserID); err != nil {
		return err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "token.revoke", "token", id, nil)
	}

	return nil
}

// ListPersonalTokens returns all personal API tokens for the current user.
func (s *Service) ListPersonalTokens(ctx context.Context) ([]APIToken, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	return s.repo.ListAPITokens(ctx, p.UserID)
}
