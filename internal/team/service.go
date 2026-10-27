package team

import (
	"context"
	"strings"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Service coordinates team operations and membership lifecycle.
type Service struct {
	repo     Repository
	archiver ProjectArchiver
	auditor  Auditor
	clock    clock.Clock
}

// New creates a new team Service.
func New(repo Repository, archiver ProjectArchiver, auditor Auditor, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{
		repo:     repo,
		archiver: archiver,
		auditor:  auditor,
		clock:    clk,
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

// CreateTeam creates a new team and records an audit event. Requires administrator role.
func (s *Service) CreateTeam(ctx context.Context, name string) (*Team, error) {
	p, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}

	now := s.clock.Now()
	t := &Team{
		WorkspaceID: p.WorkspaceID,
		Name:        name,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "team.create", "team", t.ID, map[string]any{
			"after": t,
		})
	}

	return t, nil
}

// RenameTeam renames an existing team and records an audit event. Requires administrator role.
func (s *Service) RenameTeam(ctx context.Context, id uint, name string) (*Team, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}

	before, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	after := *before
	after.Name = name
	after.UpdatedAt = now

	if err := s.repo.Update(ctx, &after); err != nil {
		return nil, err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "team.rename", "team", id, map[string]any{
			"before": before,
			"after":  &after,
		})
	}

	return &after, nil
}

// ArchiveTeam archives a team and cascades archiving to all of its projects. Requires administrator role.
func (s *Service) ArchiveTeam(ctx context.Context, id uint) error {
	if _, err := requireAdmin(ctx); err != nil {
		return err
	}

	before, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	if before.ArchivedAt == nil {
		if err := s.repo.Archive(ctx, id, now); err != nil {
			return err
		}
		if s.archiver != nil {
			if err := s.archiver.ArchiveProjectsByTeam(ctx, id, now); err != nil {
				return err
			}
		}
		if s.auditor != nil {
			_ = s.auditor.Record(ctx, "team.archive", "team", id, map[string]any{
				"before": before,
				"after": map[string]any{
					"id":          id,
					"archived_at": now,
				},
			})
		}
	}

	return nil
}

// ReplaceMembers atomically replaces a team's memberships and records an audit event. Requires administrator role.
func (s *Service) ReplaceMembers(ctx context.Context, teamID uint, members []MembershipInput) error {
	if _, err := requireAdmin(ctx); err != nil {
		return err
	}

	if _, err := s.repo.GetByID(ctx, teamID); err != nil {
		return err
	}

	seen := make(map[uint]struct{}, len(members))
	for _, m := range members {
		if m.UserID == 0 {
			return ErrUnknownUser
		}
		if _, exists := seen[m.UserID]; exists {
			return ErrDuplicateMember
		}
		seen[m.UserID] = struct{}{}
		if m.Role != "lead" && m.Role != "member" {
			return ErrInvalidRole
		}
	}

	now := s.clock.Now()
	var wsID uint
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		wsID = p.WorkspaceID
	}

	domMembers := make([]Membership, 0, len(members))
	for _, m := range members {
		domMembers = append(domMembers, Membership{
			WorkspaceID: wsID,
			TeamID:      teamID,
			UserID:      m.UserID,
			Role:        m.Role,
			CreatedAt:   now,
		})
	}

	if err := s.repo.ReplaceMemberships(ctx, teamID, domMembers); err != nil {
		return err
	}

	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "team.members.replace", "team", teamID, map[string]any{
			"members": domMembers,
		})
	}

	return nil
}

// ListTeams lists all teams with their memberships. Requires admin or lead role.
func (s *Service) ListTeams(ctx context.Context, includeArchived bool) ([]TeamWithMembers, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if !p.IsAdmin() && !p.HasRole("lead") {
		return nil, authz.ErrForbidden
	}

	return s.repo.List(ctx, includeArchived)
}

// GetByID returns the specified team. Requires admin or lead of the team.
func (s *Service) GetByID(ctx context.Context, id uint) (*Team, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if !p.IsAdmin() && !(p.HasRole("lead") && p.LeadsTeam(id)) {
		return nil, authz.ErrForbidden
	}

	return s.repo.GetByID(ctx, id)
}
