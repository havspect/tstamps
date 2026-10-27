// Package project owns the project domain entity, rules, and repository interfaces.
package project

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a project does not exist.
	ErrNotFound = errors.New("project not found")
	// ErrTeamNotActive is returned when a project belongs to or is moved to an inactive/archived team.
	ErrTeamNotActive = errors.New("team is not active")
	// ErrInvalidName is returned when a project name is empty.
	ErrInvalidName = errors.New("project name cannot be empty")
	// ErrProjectHasEntries is returned when moving a project that already has time entries.
	ErrProjectHasEntries = errors.New("project has entries")
)

// Project is the pure domain entity representing a project.
type Project struct {
	ID          uint
	WorkspaceID uint
	TeamID      uint
	Name        string
	Color       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

// AuditFields returns a map representation of the project for audit logging.
func (p *Project) AuditFields() map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"id":           p.ID,
		"workspace_id": p.WorkspaceID,
		"team_id":      p.TeamID,
		"name":         p.Name,
		"color":        p.Color,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
		"archived_at":  p.ArchivedAt,
	}
}
