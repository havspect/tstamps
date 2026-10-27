package entry

import (
	"context"
	"time"
)

// ListFilter narrows entries listed by the repository.
type ListFilter struct {
	UserID         uint
	ProjectID      uint
	ProjectIDs     []uint
	From           *time.Time
	To             *time.Time
	IncludeDeleted bool
}

// Repository defines persistence operations for entries.
type Repository interface {
	GetByID(ctx context.Context, id uint) (*Entry, error)
	GetRunning(ctx context.Context, userID uint) (*Entry, error)
	List(ctx context.Context, filter ListFilter) ([]Entry, error)
	Create(ctx context.Context, e *Entry) error
	Update(ctx context.Context, e *Entry) error
	SoftDelete(ctx context.Context, id uint, deletedAt time.Time) error
	Restore(ctx context.Context, id uint) error
}

// UnitOfWork runs operations inside a transactional boundary.
type UnitOfWork interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditRecorder records an immutable audit log entry.
type AuditRecorder interface {
	Record(ctx context.Context, action string, targetType string, targetID uint, changes any) error
}

// ProjectChecker verifies project validity and availability for time tracking.
type ProjectChecker interface {
	IsProjectLoggable(ctx context.Context, projectID uint) (bool, error)
}
