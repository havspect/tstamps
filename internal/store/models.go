package store

import "time"

// User is a person who can log in; Email is stored lowercased.
type User struct {
	ID           uint   `gorm:"primarykey"`
	Email        string `gorm:"uniqueIndex;not null"`
	Name         string `gorm:"not null"`
	PasswordHash string `gorm:"not null"`
	Timezone     string `gorm:"not null;default:UTC"`
	IsAdmin      bool   `gorm:"not null;default:false"`
	Active       bool   `gorm:"not null;default:true"`
	// MustChangePassword confines the user to /me, /me/password and /auth/logout.
	MustChangePassword bool `gorm:"not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Session is a login session, stored as a hash of its opaque token.
type Session struct {
	ID        uint      `gorm:"primarykey"`
	UserID    uint      `gorm:"index;not null"`
	TokenHash string    `gorm:"uniqueIndex;not null"` // sha256 hex, never the raw token
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}

// Membership roles. A user has at most one membership per team.
const (
	RoleLead   = "lead"
	RoleMember = "member"
)

// Team is a named group that owns projects and has leads. Archived teams grant nothing.
type Team struct {
	ID         uint   `gorm:"primarykey"`
	Name       string `gorm:"not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
}

// TeamMembership links a user to a team with the role lead or member.
type TeamMembership struct {
	ID        uint   `gorm:"primarykey"`
	TeamID    uint   `gorm:"not null;uniqueIndex:idx_team_user"`
	UserID    uint   `gorm:"not null;uniqueIndex:idx_team_user"`
	Role      string `gorm:"not null;default:member"`
	CreatedAt time.Time
}

// Project is the unit of work entries are logged against. It belongs to one team.
type Project struct {
	ID        uint   `gorm:"primarykey"`
	TeamID    uint   `gorm:"not null;index"`
	Name      string `gorm:"not null"`
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
	// Archived projects leave the pickers but keep their hours.
	ArchivedAt *time.Time
}

// Entry is one continuous stretch of work on one project. EndAt is nil while it
// runs; DeletedAt is set instead of removing the row. Timezone is the IANA zone
// the entry was logged in, and decides its day and week in reports.
type Entry struct {
	ID        uint       `gorm:"primarykey"`
	UserID    uint       `gorm:"not null;index"`
	ProjectID uint       `gorm:"not null;index"`
	StartAt   time.Time  `gorm:"not null;index"`
	EndAt     *time.Time `gorm:"index"`
	Timezone  string     `gorm:"not null;default:''"`
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
}

// AuditEvent is one change made by an actor whose authority came from a role.
// Changes is JSON {"before":{...},"after":{...}}; hashes never enter it.
type AuditEvent struct {
	ID           uint      `gorm:"primarykey"`
	At           time.Time `gorm:"not null;index"`
	ActorUserID  uint      `gorm:"not null;index"`
	ActorTokenID *uint
	ActorRole    string `gorm:"not null"`
	Action       string `gorm:"not null;index"`
	TargetType   string `gorm:"not null;index:idx_audit_target"`
	TargetID     uint   `gorm:"not null;index:idx_audit_target"`
	Changes      string `gorm:"not null"`
}

// APIToken is a personal token that acts as its user; only the hash is stored.
type APIToken struct {
	ID         uint   `gorm:"primarykey"`
	UserID     uint   `gorm:"not null;index"`
	Name       string `gorm:"not null"`
	TokenHash  string `gorm:"uniqueIndex;not null"` // sha256 hex, never the raw token
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}
