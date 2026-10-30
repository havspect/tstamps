package entry

import (
	"context"
	"errors"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Service provides entry domain operations enforcing authorization and invariant rules.
type Service struct {
	repo       Repository
	uow        UnitOfWork
	audit      AuditRecorder
	projects   ProjectChecker
	authorizer authz.Authorizer
	clock      clock.Clock
}

// NewService creates a new entry domain Service.
func NewService(
	repo Repository,
	uow UnitOfWork,
	audit AuditRecorder,
	projects ProjectChecker,
	authorizer authz.Authorizer,
	clk clock.Clock,
) *Service {
	return &Service{
		repo:       repo,
		uow:        uow,
		audit:      audit,
		projects:   projects,
		authorizer: authorizer,
		clock:      clk,
	}
}

// LogEntry logs a completed time entry.
func (s *Service) LogEntry(
	ctx context.Context,
	projectID uint,
	start, end time.Time,
	tz, note string,
) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionLogEntry, authz.EntryResource{UserID: p.UserID, ProjectID: projectID}); err != nil {
		return nil, err
	}

	loggable, err := s.projects.IsProjectLoggable(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !loggable {
		return nil, ErrProjectNotLoggable
	}

	now := s.clock.Now()
	if err := Validate(start, end, now); err != nil {
		return nil, err
	}
	if tz == "" {
		tz = "UTC"
	}

	e := &Entry{
		WorkspaceID: p.WorkspaceID,
		UserID:      p.UserID,
		ProjectID:   projectID,
		StartAt:     start,
		EndAt:       &end,
		Timezone:    tz,
		Note:        note,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		if err := s.ensureNoOverlap(txCtx, e); err != nil {
			return err
		}
		return s.repo.Create(txCtx, e)
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// PatchEntry modifies an existing live entry.
func (s *Service) PatchEntry(
	ctx context.Context,
	id uint,
	projectID *uint,
	start, end *time.Time,
	timezone *string,
	note *string,
) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	var updated *Entry
	err := s.uow.Run(ctx, func(txCtx context.Context) error {
		e, err := s.repo.GetByID(txCtx, id)
		if err != nil || e == nil || e.DeletedAt != nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionModifyEntry, authz.EntryResource{UserID: e.UserID, ProjectID: e.ProjectID}); err != nil {
			return err
		}

		if projectID != nil && *projectID != e.ProjectID {
			loggable, err := s.projects.IsProjectLoggable(txCtx, *projectID)
			if err != nil {
				return err
			}
			if !loggable {
				return ErrProjectNotLoggable
			}
			e.ProjectID = *projectID
		}

		if note != nil {
			e.Note = *note
		}

		if timezone != nil {
			if _, err := time.LoadLocation(*timezone); err != nil {
				return errors.New("invalid timezone")
			}
			e.Timezone = *timezone
		}

		startVal := e.StartAt
		if start != nil {
			startVal = *start
		}
		var endVal time.Time
		switch {
		case end != nil:
			endVal = *end
		case e.EndAt != nil:
			endVal = *e.EndAt
		default:
			if start != nil {
				return ErrRetimeRunning
			}
		}

		now := s.clock.Now()
		if !endVal.IsZero() && (start != nil || end != nil) {
			if err := Validate(startVal, endVal, now); err != nil {
				return err
			}
			e.StartAt = startVal
			e.EndAt = &endVal
		}
		e.UpdatedAt = now

		if projectID != nil || start != nil || end != nil {
			if err := s.ensureNoOverlap(txCtx, e); err != nil {
				return err
			}
		}

		if err := s.repo.Update(txCtx, e); err != nil {
			return err
		}
		updated = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteEntry soft-deletes a live entry.
func (s *Service) DeleteEntry(ctx context.Context, id uint) error {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return authz.ErrUnauthorized
	}

	return s.uow.Run(ctx, func(txCtx context.Context) error {
		e, err := s.repo.GetByID(txCtx, id)
		if err != nil || e == nil || e.DeletedAt != nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionDeleteEntry, authz.EntryResource{UserID: e.UserID, ProjectID: e.ProjectID}); err != nil {
			return err
		}

		return s.repo.SoftDelete(txCtx, id, s.clock.Now())
	})
}

// RestoreEntry restores a soft-deleted entry (admin only, audited).
func (s *Service) RestoreEntry(ctx context.Context, id uint) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	var restored *Entry
	err := s.uow.Run(ctx, func(txCtx context.Context) error {
		before, err := s.repo.GetByID(txCtx, id)
		if err != nil || before == nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionRestoreEntry, authz.EntryResource{UserID: before.UserID, ProjectID: before.ProjectID}); err != nil {
			return err
		}

		if err := s.ensureNoOverlap(txCtx, before); err != nil {
			return err
		}

		if err := s.repo.Restore(txCtx, id); err != nil {
			return err
		}

		after, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		if s.audit != nil {
			if err := s.audit.Record(txCtx, "entry.restore", "entry", id, map[string]any{
				"before": before.AuditFields(),
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

// StartTimer starts a running timer for the authenticated user.
func (s *Service) StartTimer(ctx context.Context, projectID uint, tz, note string) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionLogEntry, authz.EntryResource{UserID: p.UserID, ProjectID: projectID}); err != nil {
		return nil, err
	}

	loggable, err := s.projects.IsProjectLoggable(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !loggable {
		return nil, ErrProjectNotLoggable
	}

	running, err := s.repo.GetRunning(ctx, p.UserID)
	if err == nil && running != nil {
		return nil, &AlreadyRunning{Entry: running}
	}

	now := s.clock.Now()
	if tz == "" {
		tz = "UTC"
	}

	e := &Entry{
		WorkspaceID: p.WorkspaceID,
		UserID:      p.UserID,
		ProjectID:   projectID,
		StartAt:     now,
		EndAt:       nil,
		Timezone:    tz,
		Note:        note,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		if err := s.ensureNoOverlap(txCtx, e); err != nil {
			return err
		}
		createErr := s.repo.Create(txCtx, e)
		if createErr != nil {
			if errors.Is(createErr, ErrRunningExists) {
				if cur, rErr := s.repo.GetRunning(txCtx, p.UserID); rErr == nil && cur != nil {
					return &AlreadyRunning{Entry: cur}
				}
			}
			return createErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// StopTimer stops the user's running entry.
func (s *Service) StopTimer(ctx context.Context, note *string) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	var stopped *Entry
	err := s.uow.Run(ctx, func(txCtx context.Context) error {
		running, err := s.repo.GetRunning(txCtx, p.UserID)
		if err != nil || running == nil {
			return ErrNotFound
		}

		if err := s.authorizer.Check(txCtx, p, authz.ActionModifyEntry, authz.EntryResource{UserID: running.UserID, ProjectID: running.ProjectID}); err != nil {
			return err
		}

		now := s.clock.Now()
		if err := Validate(running.StartAt, now, now); err != nil {
			return err
		}

		running.EndAt = &now
		if note != nil && *note != "" {
			running.Note = *note
		}
		running.UpdatedAt = now

		if err := s.ensureNoOverlap(txCtx, running); err != nil {
			return err
		}

		if err := s.repo.Update(txCtx, running); err != nil {
			return err
		}
		stopped = running
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stopped, nil
}

// GetRunning returns the user's current running timer, or nil if none.
func (s *Service) GetRunning(ctx context.Context) (*Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	running, err := s.repo.GetRunning(ctx, p.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return running, nil
}

// ensureNoOverlap rejects an entry that shares any time with another of the
// user's live entries on the same project. Time on other projects may overlap.
func (s *Service) ensureNoOverlap(ctx context.Context, e *Entry) error {
	filter := ListFilter{UserID: e.UserID, ProjectID: e.ProjectID, From: &e.StartAt, To: e.EndAt}
	existing, err := s.repo.List(ctx, filter)
	if err != nil {
		return err
	}
	for _, other := range existing {
		if other.ID != e.ID {
			return ErrOverlap
		}
	}
	return nil
}

// ListEntries returns entries matching filter within user's permissions.
func (s *Service) ListEntries(ctx context.Context, filter ListFilter) ([]Entry, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}

	if filter.IncludeDeleted && !p.IsAdmin() {
		return nil, authz.ErrForbidden
	}

	if !p.IsAdmin() && !p.HasRole("lead") {
		filter.UserID = p.UserID
	}

	return s.repo.List(ctx, filter)
}
