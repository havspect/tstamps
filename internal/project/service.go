package project

import (
	"context"
	"strings"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Service provides project domain operations enforcing authorization and invariant rules.
type Service struct {
	repo       Repository
	uow        UnitOfWork
	audit      AuditRecorder
	teams      TeamChecker
	authorizer authz.Authorizer
	clock      clock.Clock
}

// NewService creates a new project domain Service.
func NewService(
	repo Repository,
	uow UnitOfWork,
	audit AuditRecorder,
	teams TeamChecker,
	authorizer authz.Authorizer,
	clk clock.Clock,
) *Service {
	return &Service{
		repo:       repo,
		uow:        uow,
		audit:      audit,
		teams:      teams,
		authorizer: authorizer,
		clock:      clk,
	}
}

// CreateProject creates a new project under the given team (audited).
func (s *Service) CreateProject(ctx context.Context, teamID uint, name, color string) (*Project, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	if err := s.authorizer.Check(ctx, p, authz.ActionManageProject, authz.ProjectResource{TeamID: teamID}); err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) == "" {
		return nil, ErrInvalidName
	}

	active, err := s.teams.IsTeamActive(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrTeamNotActive
	}

	now := s.clock.Now()
	proj := &Project{
		WorkspaceID: p.WorkspaceID,
		TeamID:      teamID,
		Name:        name,
		Color:       color,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, proj); err != nil {
			return err
		}
		if s.audit != nil {
			return s.audit.Record(txCtx, "project.create", "project", proj.ID, map[string]any{
				"after": proj.AuditFields(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return proj, nil
}

// UpdateProject updates fields of a project, and verifies HasEntries==false if the team changes (audited).
func (s *Service) UpdateProject(ctx context.Context, id uint, teamID *uint, name, color *string, archived *bool) (*Project, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	var updated *Project
	err := s.uow.Run(ctx, func(txCtx context.Context) error {
		before, err := s.repo.GetByID(txCtx, id)
		if err != nil || before == nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionManageProject, authz.ProjectResource{
			TeamID:   before.TeamID,
			Archived: before.ArchivedAt != nil,
		}); err != nil {
			return err
		}

		beforeAudit := before.AuditFields()

		if teamID != nil && *teamID != before.TeamID {
			if err := s.authorizer.Check(txCtx, p, authz.ActionManageProject, authz.ProjectResource{TeamID: *teamID}); err != nil {
				return err
			}

			active, err := s.teams.IsTeamActive(txCtx, *teamID)
			if err != nil {
				return err
			}
			if !active {
				return ErrTeamNotActive
			}

			hasEntries, err := s.repo.HasEntries(txCtx, id)
			if err != nil {
				return err
			}
			if hasEntries {
				return ErrProjectHasEntries
			}
			before.TeamID = *teamID
		}

		if name != nil {
			if strings.TrimSpace(*name) == "" {
				return ErrInvalidName
			}
			before.Name = *name
		}

		if color != nil {
			before.Color = *color
		}

		if archived != nil {
			if *archived {
				at := s.clock.Now()
				before.ArchivedAt = &at
			} else {
				before.ArchivedAt = nil
			}
		}

		before.UpdatedAt = s.clock.Now()

		if err := s.repo.Update(txCtx, before); err != nil {
			return err
		}

		after, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		if s.audit != nil {
			if err := s.audit.Record(txCtx, "project.update", "project", id, map[string]any{
				"before": beforeAudit,
				"after":  after.AuditFields(),
			}); err != nil {
				return err
			}
		}

		updated = after
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ArchiveProject archives a project (audited).
func (s *Service) ArchiveProject(ctx context.Context, id uint) error {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return authz.ErrUnauthorized
	}

	return s.uow.Run(ctx, func(txCtx context.Context) error {
		before, err := s.repo.GetByID(txCtx, id)
		if err != nil || before == nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionManageProject, authz.ProjectResource{
			TeamID:   before.TeamID,
			Archived: before.ArchivedAt != nil,
		}); err != nil {
			return err
		}

		beforeAudit := before.AuditFields()
		now := s.clock.Now()
		if err := s.repo.Archive(txCtx, id, now); err != nil {
			return err
		}

		after, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		if s.audit != nil {
			return s.audit.Record(txCtx, "project.archive", "project", id, map[string]any{
				"before": beforeAudit,
				"after":  after.AuditFields(),
			})
		}
		return nil
	})
}

// RestoreProject restores an archived project, verifying the owning team is active (admin only, audited).
func (s *Service) RestoreProject(ctx context.Context, id uint) (*Project, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	var restored *Project
	err := s.uow.Run(ctx, func(txCtx context.Context) error {
		before, err := s.repo.GetByID(txCtx, id)
		if err != nil || before == nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionRestoreProject, authz.ProjectResource{
			TeamID:   before.TeamID,
			Archived: true,
		}); err != nil {
			return err
		}

		active, err := s.teams.IsTeamActive(txCtx, before.TeamID)
		if err != nil {
			return err
		}
		if !active {
			return ErrTeamNotActive
		}

		beforeAudit := before.AuditFields()
		if err := s.repo.Restore(txCtx, id); err != nil {
			return err
		}

		after, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		if s.audit != nil {
			if err := s.audit.Record(txCtx, "project.restore", "project", id, map[string]any{
				"before": beforeAudit,
				"after":  after.AuditFields(),
			}); err != nil {
				return err
			}
		}

		restored = after
		return nil
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}

// ListProjects lists projects according to permissions and archive filter.
func (s *Service) ListProjects(ctx context.Context, includeArchived bool) ([]Project, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	if includeArchived && !p.IsAdmin() {
		return nil, authz.ErrForbidden
	}

	filter := ListFilter{
		IncludeArchived: includeArchived,
	}
	return s.repo.List(ctx, filter)
}

// GetByID returns the project by id if viewable by the user.
func (s *Service) GetByID(ctx context.Context, id uint) (*Project, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	proj, err := s.repo.GetByID(ctx, id)
	if err != nil || proj == nil {
		return nil, ErrNotFound
	}

	if err := s.authorizer.Check(ctx, p, authz.ActionViewProject, authz.ProjectResource{
		TeamID:   proj.TeamID,
		Archived: proj.ArchivedAt != nil,
	}); err != nil {
		return nil, err
	}

	return proj, nil
}
