package change

import (
	"context"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// Projects manages mutations to projects.
type Projects struct {
	store *store.Store
	clock clock.Clock
}

// ProjectPatch holds optional fields for updating a project.
type ProjectPatch struct {
	Name     *string
	Color    *string
	TeamID   *uint
	Archived *bool
}

// Create inserts a project and records an audit event in one transaction.
func (s *Projects) Create(ctx context.Context, actor Actor, teamID uint, name, color string) (*store.Project, error) {
	now := s.clock.Now()
	p := &store.Project{TeamID: teamID, Name: name, Color: color, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(func(tx *store.Store) error {
		if err := tx.CreateProject(p); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "project.create", "project", p.ID,
			store.Change{After: p.AuditFields()})
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Update mutates project fields, handles moving to another team, and records an audit event in one transaction.
func (s *Projects) Update(ctx context.Context, actor Actor, id uint, patch ProjectPatch) (*store.Project, error) {
	now := s.clock.Now()
	var after *store.Project

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.ProjectByID(id)
		if err != nil {
			return err
		}
		p := *before
		if patch.TeamID != nil && *patch.TeamID != p.TeamID {
			if err := tx.MoveProject(p.ID, *patch.TeamID, now); err != nil {
				return err
			}
			p.TeamID = *patch.TeamID
		}
		if patch.Archived != nil {
			if *patch.Archived {
				at := now
				p.ArchivedAt = &at
			} else {
				p.ArchivedAt = nil
			}
		}
		if patch.Name != nil {
			p.Name = *patch.Name
		}
		if patch.Color != nil {
			p.Color = *patch.Color
		}
		if err := tx.UpdateProject(&p); err != nil {
			return err
		}
		if after, err = tx.ProjectByID(id); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "project.update", "project", id,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})

	if err != nil {
		return nil, err
	}
	return after, nil
}

// Restore unarchives project id and records an audit event in one transaction.
func (s *Projects) Restore(ctx context.Context, actor Actor, id uint) (*store.Project, error) {
	now := s.clock.Now()
	var after *store.Project

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.ProjectByID(id)
		if err != nil {
			return err
		}
		p := *before
		p.ArchivedAt = nil
		if err := tx.UpdateProject(&p); err != nil {
			return err
		}
		if after, err = tx.ProjectByID(id); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "project.restore", "project", id,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})

	if err != nil {
		return nil, err
	}
	return after, nil
}
