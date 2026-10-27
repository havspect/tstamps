// Package team manages team definitions, memberships, and team project lifecycles.
package team

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates the team does not exist.
	ErrNotFound = errors.New("team not found")

	// ErrUnknownUser indicates a membership references a user that does not exist.
	ErrUnknownUser = errors.New("unknown user")

	// ErrDuplicateMember indicates the same user is specified more than once.
	ErrDuplicateMember = errors.New("duplicate user in memberships")

	// ErrInvalidName indicates an empty or invalid team name was supplied.
	ErrInvalidName = errors.New("team name cannot be empty")

	// ErrInvalidRole indicates an unaccepted membership role (only 'lead' and 'member' are valid).
	ErrInvalidRole = errors.New("invalid role")
)

// Team represents a group within a workspace that owns projects and memberships.
type Team struct {
	ID          uint
	WorkspaceID uint
	Name        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

// Membership defines a user's role within a team.
type Membership struct {
	ID          uint
	WorkspaceID uint
	TeamID      uint
	UserID      uint
	Role        string // "lead" or "member"
	CreatedAt   time.Time
}

// TeamWithMembers pairs a team with its current memberships.
type TeamWithMembers struct {
	Team    Team
	Members []Membership
}

// MembershipInput specifies user ID and role for setting memberships.
type MembershipInput struct {
	UserID uint
	Role   string
}

// Repository defines storage operations for teams and memberships.
type Repository interface {
	GetByID(ctx context.Context, id uint) (*Team, error)
	List(ctx context.Context, includeArchived bool) ([]TeamWithMembers, error)
	Create(ctx context.Context, t *Team) error
	Update(ctx context.Context, t *Team) error
	Archive(ctx context.Context, id uint, at time.Time) error
	ReplaceMemberships(ctx context.Context, teamID uint, members []Membership) error
}

// ProjectArchiver defines cascading archive operations on team projects.
type ProjectArchiver interface {
	ArchiveProjectsByTeam(ctx context.Context, teamID uint, at time.Time) error
}

// Auditor defines the audit recording interface.
type Auditor interface {
	Record(ctx context.Context, action, targetType string, targetID uint, changes any) error
}
