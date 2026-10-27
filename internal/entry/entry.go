// Package entry owns the entry and timer domain rules, entities, and services.
package entry

import (
	"errors"
	"time"
)

var (
	// ErrInvalidRange is returned when end is not after start.
	ErrInvalidRange = errors.New("end must be after start")
	// ErrTooLong is returned when an entry duration exceeds 24 hours.
	ErrTooLong = errors.New("entry is longer than 24 hours")
	// ErrFuture is returned when start or end is too far in the future.
	ErrFuture = errors.New("entry is too far in the future")
	// ErrNotFound is returned when an entry cannot be found.
	ErrNotFound = errors.New("entry not found")
	// ErrProjectNotLoggable is returned when an entry points at an unknown or archived project.
	ErrProjectNotLoggable = errors.New("project is not loggable")
	// ErrUnknownProject is returned when an entry points at a project that does not exist.
	ErrUnknownProject = errors.New("unknown project")
	// ErrArchivedProject is returned when an entry points at an archived project.
	ErrArchivedProject = errors.New("project is archived")
	// ErrRetimeRunning is returned when a running entry's start moves without an end.
	ErrRetimeRunning = errors.New("a running entry needs an end_at to be retimed")
	// ErrRunningExists is returned when a write would leave a user with a second running entry.
	ErrRunningExists = errors.New("user already has a running entry")
	// ErrForbidden is returned when an entry belongs to another user.
	ErrForbidden = errors.New("entry belongs to another user")
)

const (
	// MaxLength is the maximum duration for a single entry.
	MaxLength = 24 * time.Hour
	// FutureGrace is the allowable time ahead of now.
	FutureGrace = 5 * time.Minute
)

// Entry is the pure domain entity for one continuous stretch of work.
type Entry struct {
	ID          uint
	WorkspaceID uint
	UserID      uint
	ProjectID   uint
	StartAt     time.Time
	EndAt       *time.Time
	Timezone    string
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// AuditFields returns a map representation of the entry for audit logging.
func (e *Entry) AuditFields() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"id":           e.ID,
		"workspace_id": e.WorkspaceID,
		"user_id":      e.UserID,
		"project_id":   e.ProjectID,
		"start_at":     e.StartAt,
		"end_at":       e.EndAt,
		"timezone":     e.Timezone,
		"note":         e.Note,
		"created_at":   e.CreatedAt,
		"updated_at":   e.UpdatedAt,
		"deleted_at":   e.DeletedAt,
	}
}

// AlreadyRunning is returned when the user already has a running entry.
type AlreadyRunning struct {
	Entry *Entry
}

func (e *AlreadyRunning) Error() string {
	return "user already has a running entry"
}

// Validate checks a finished entry's start and end against now.
func Validate(start, end, now time.Time) error {
	if !end.After(start) {
		return ErrInvalidRange
	}
	if end.Sub(start) > MaxLength {
		return ErrTooLong
	}
	if start.After(now.Add(FutureGrace)) || end.After(now.Add(FutureGrace)) {
		return ErrFuture
	}
	return nil
}
