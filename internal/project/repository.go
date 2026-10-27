package project

import (
	"context"
	"time"
)

// ListFilter narrows projects listed by the repository.
type ListFilter struct {
	TeamID          uint
	TeamIDs         []uint
	IncludeArchived bool
}

// Repository defines persistence operations for projects.
type Repository interface {
	GetByID(ctx context.Context, id uint) (*Project, error)
	List(ctx context.Context, filter ListFilter) ([]Project, error)
	Create(ctx context.Context, p *Project) error
	Update(ctx context.Context, p *Project) error
	Archive(ctx context.Context, id uint, at time.Time) error
	Restore(ctx context.Context, id uint) error
	HasEntries(ctx context.Context, id uint) (bool, error)
}

// TeamChecker verifies whether a team is active and exists.
type TeamChecker interface {
	IsTeamActive(ctx context.Context, teamID uint) (bool, error)
}

// UnitOfWork runs operations inside a transactional boundary.
type UnitOfWork interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditRecorder records an immutable audit log entry.
type AuditRecorder interface {
	Record(ctx context.Context, action string, targetType string, targetID uint, changes any) error
}
