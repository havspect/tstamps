// Package user provides domain models and services for identity, authentication credentials, and user lifecycle.
package user

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates the user does not exist.
	ErrNotFound = errors.New("user not found")

	// ErrLastAdmin indicates an update would leave the workspace without any active administrator.
	ErrLastAdmin = errors.New("last admin")

	// ErrDuplicateEmail indicates another user already exists with the specified email.
	ErrDuplicateEmail = errors.New("email already exists")

	// ErrInvalidEmail indicates an empty or invalid email address.
	ErrInvalidEmail = errors.New("invalid email")

	// ErrInvalidName indicates an empty user name.
	ErrInvalidName = errors.New("name cannot be empty")

	// ErrInvalidTokenName indicates an empty API token name.
	ErrInvalidTokenName = errors.New("token name cannot be empty")

	// ErrInvalidPassword indicates the provided current password was incorrect.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrPasswordTooShort indicates the new password has fewer than 8 characters.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")

	// ErrInvalidWeeklyHours indicates capacity outside 0–168 hours.
	ErrInvalidWeeklyHours = errors.New("weekly hours must be between 0 and 168")
)

// User represents an identity within a workspace.
type User struct {
	ID                 uint
	WorkspaceID        uint
	Email              string
	Name               string
	PasswordHash       string
	Timezone           string
	WeeklyHours        int
	IsAdmin            bool
	Active             bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// APIToken represents a personal access token for automated and CLI access.
type APIToken struct {
	ID          uint
	WorkspaceID uint
	UserID      uint
	Name        string
	TokenHash   string
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

// Session represents an active login session.
type Session struct {
	ID          uint
	WorkspaceID uint
	UserID      uint
	TokenHash   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// Profile pairs user details with the teams they lead.
type Profile struct {
	User       User
	LedTeamIDs []uint
}

// CreateInput contains parameters for creating a new user.
type CreateInput struct {
	Email       string
	Name        string
	Timezone    string
	WeeklyHours int
	IsAdmin     bool
}

// UpdateInput contains parameters for updating user details.
type UpdateInput struct {
	ID          uint
	Name        *string
	Timezone    *string
	WeeklyHours *int
	IsAdmin     *bool
	Active      *bool
}

// Repository defines storage operations for users, sessions, and tokens.
type Repository interface {
	GetByID(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context) ([]User, error)
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	CountActiveAdmins(ctx context.Context) (int, error)
	SaveSession(ctx context.Context, s *Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessionsExcept(ctx context.Context, userID uint, keepTokenHash string) error
	CreateAPIToken(ctx context.Context, t *APIToken) error
	GetAPITokenByTokenHash(ctx context.Context, tokenHash string) (*APIToken, error)
	TouchAPIToken(ctx context.Context, id uint, at time.Time) error
	RevokeAPIToken(ctx context.Context, id uint, userID uint) error
	ListAPITokens(ctx context.Context, userID uint) ([]APIToken, error)
	RevokeAllOAuthTokens(ctx context.Context, userID uint) error
}

// TimerStopper handles stopping running entry timers when a user is deactivated.
type TimerStopper interface {
	StopRunningTimer(ctx context.Context, userID uint, note string) error
}

// Auditor defines the audit recording interface.
type Auditor interface {
	Record(ctx context.Context, action, targetType string, targetID uint, changes any) error
}
