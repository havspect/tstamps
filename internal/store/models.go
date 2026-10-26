package store

import "time"

// Workspace is one company in a shared database. Its ID scopes every tenant row.
type Workspace struct {
	ID        uint   `gorm:"primarykey"`
	Slug      string `gorm:"uniqueIndex;not null"`
	CreatedAt time.Time
}

// PendingSignup is an emailed signup link that has not been used. Only the token
// hash is stored; it belongs to no workspace yet.
type PendingSignup struct {
	TokenHash string `gorm:"primaryKey"`
	Email     string `gorm:"not null"`
	CreatedAt time.Time
}

// User is a person who can log in; Email is stored lowercased and unique within
// one workspace.
type User struct {
	ID                 uint   `gorm:"primarykey"`
	WorkspaceID        uint   `gorm:"not null;uniqueIndex:idx_users_workspace_email,priority:1"`
	Email              string `gorm:"not null;uniqueIndex:idx_users_workspace_email,priority:2"`
	Name               string `gorm:"not null"`
	PasswordHash       string `gorm:"not null"`
	Timezone           string `gorm:"not null;default:UTC"`
	IsAdmin            bool   `gorm:"not null;default:false"`
	Active             bool   `gorm:"not null;default:true"`
	MustChangePassword bool   `gorm:"not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Session is a login session, stored as a hash of its opaque token.
type Session struct {
	ID          uint      `gorm:"primarykey"`
	WorkspaceID uint      `gorm:"not null;index"`
	UserID      uint      `gorm:"index;not null"`
	TokenHash   string    `gorm:"uniqueIndex;not null"` // sha256 hex, never the raw token
	ExpiresAt   time.Time `gorm:"not null"`
	CreatedAt   time.Time
}

// APIToken is a personal access token for CLI or API use, stored as a hash.
type APIToken struct {
	ID          uint       `gorm:"primarykey"`
	WorkspaceID uint       `gorm:"not null;index"`
	UserID      uint       `gorm:"index;not null"`
	Name        string     `gorm:"not null"`
	TokenHash   string     `gorm:"uniqueIndex;not null"`
	ExpiresAt   time.Time  `gorm:"not null"`
	LastUsedAt  *time.Time
	RevokedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
}

// Membership roles. A user has at most one membership per team.
const (
	RoleLead   = "lead"
	RoleMember = "member"
)

// Team is a named group that owns projects and has leads. Archived teams grant nothing.
type Team struct {
	ID          uint   `gorm:"primarykey"`
	WorkspaceID uint   `gorm:"not null;index"`
	Name        string `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

// TeamMembership links a user to a team with the role lead or member.
type TeamMembership struct {
	ID          uint   `gorm:"primarykey"`
	WorkspaceID uint   `gorm:"not null;index"`
	TeamID      uint   `gorm:"not null;uniqueIndex:idx_team_user"`
	UserID      uint   `gorm:"not null;uniqueIndex:idx_team_user"`
	Role        string `gorm:"not null;default:member"`
	CreatedAt   time.Time
}

// Project is the unit of work entries are logged against. It belongs to one team.
type Project struct {
	ID          uint   `gorm:"primarykey"`
	WorkspaceID uint   `gorm:"not null;index"`
	TeamID      uint   `gorm:"not null;index"`
	Name        string `gorm:"not null"`
	Color       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// Archived projects leave the pickers but keep their hours.
	ArchivedAt *time.Time
}

// Entry is one continuous stretch of work on one project. EndAt is nil while it
// runs; DeletedAt is set instead of removing the row. Timezone is the IANA zone
// the entry was logged in, and decides its day and week in reports.
type Entry struct {
	ID          uint       `gorm:"primarykey"`
	WorkspaceID uint       `gorm:"not null;index"`
	UserID      uint       `gorm:"not null;index"`
	ProjectID   uint       `gorm:"not null;index"`
	StartAt     time.Time  `gorm:"not null;index"`
	EndAt       *time.Time `gorm:"index"`
	Timezone    string     `gorm:"not null;default:''"`
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time `gorm:"index"`
}

// AuditEvent is one record in the workspace's tamper-evident audit log.
type AuditEvent struct {
	ID           uint      `gorm:"primarykey"`
	WorkspaceID  uint      `gorm:"not null;index"`
	At           time.Time `gorm:"not null;index"`
	ActorUserID  uint      `gorm:"not null;index"`
	ActorTokenID *uint     `gorm:"index"`
	ActorRole    string    `gorm:"not null"`
	Action       string    `gorm:"not null"`
	TargetType   string    `gorm:"not null"`
	TargetID     uint      `gorm:"not null"`
	Changes      string    `gorm:"not null"` // JSON {before, after}
	CreatedAt    time.Time
}

// ServerSecret stores the server-wide Fosite HMAC secret (row ID=1).
type ServerSecret struct {
	ID        uint      `gorm:"primaryKey"`
	Secret    []byte    `gorm:"not null"`
	CreatedAt time.Time
}

// OAuthClient stores OAuth2 clients per workspace.
type OAuthClient struct {
	ID               uint   `gorm:"primarykey"`
	WorkspaceID      uint   `gorm:"not null;uniqueIndex:idx_oauth_clients_ws_cid,priority:1"`
	ClientID         string `gorm:"not null;uniqueIndex:idx_oauth_clients_ws_cid,priority:2"`
	ClientName       string `gorm:"not null"`
	ClientSecretHash string
	RedirectURIs     string `gorm:"not null"` // JSON []string
	GrantTypes       string `gorm:"not null"` // JSON []string
	ResponseTypes    string `gorm:"not null"` // JSON []string
	Scopes           string `gorm:"not null"` // JSON []string
	Public           bool   `gorm:"not null;default:true"`
	CreatedAt        time.Time
}

// OAuthCode stores authorization codes.
type OAuthCode struct {
	ID            uint      `gorm:"primarykey"`
	WorkspaceID   uint      `gorm:"not null;index"`
	SignatureHash string    `gorm:"uniqueIndex;not null"`
	RequestID     string    `gorm:"not null;index"`
	ClientID      string    `gorm:"not null;index"`
	UserID        uint      `gorm:"not null;index"`
	SessionData   string    `gorm:"not null"`
	PKCEActive    bool      `gorm:"not null;default:false"`
	Used          bool      `gorm:"not null;default:false"`
	ExpiresAt     time.Time `gorm:"not null;index"`
	CreatedAt     time.Time
}

// OAuthAccessToken stores access tokens.
type OAuthAccessToken struct {
	ID            uint       `gorm:"primarykey"`
	WorkspaceID   uint       `gorm:"not null;index"`
	SignatureHash string     `gorm:"uniqueIndex;not null"`
	RequestID     string     `gorm:"not null;index"`
	ClientID      string     `gorm:"not null;index"`
	UserID        uint       `gorm:"not null;index"`
	Scopes        string     `gorm:"not null"` // JSON []string
	Audience      string     `gorm:"not null"` // JSON []string
	SessionData   string     `gorm:"not null"`
	ExpiresAt     time.Time  `gorm:"not null;index"`
	LastUsedAt    *time.Time
	RevokedAt     *time.Time `gorm:"index"`
	CreatedAt     time.Time
}

// OAuthRefreshToken stores refresh tokens.
type OAuthRefreshToken struct {
	ID                  uint       `gorm:"primarykey"`
	WorkspaceID         uint       `gorm:"not null;index"`
	SignatureHash       string     `gorm:"uniqueIndex;not null"`
	AccessSignatureHash string     `gorm:"not null;index"`
	RequestID           string     `gorm:"not null;index"`
	ClientID            string     `gorm:"not null;index"`
	UserID              uint       `gorm:"not null;index"`
	Scopes              string     `gorm:"not null"` // JSON []string
	SessionData         string     `gorm:"not null"`
	Active              bool       `gorm:"not null;default:true"`
	ExpiresAt           time.Time  `gorm:"not null;index"`
	LastUsedAt          *time.Time
	RevokedAt           *time.Time `gorm:"index"`
	CreatedAt           time.Time
}

// OAuthDeviceCode stores device authorization codes.
type OAuthDeviceCode struct {
	ID             uint       `gorm:"primarykey"`
	WorkspaceID    uint       `gorm:"not null;index"`
	DeviceCodeHash string     `gorm:"uniqueIndex;not null"`
	UserCodeHash   string     `gorm:"not null;uniqueIndex:idx_oauth_device_ws_ucode,priority:2"`
	RequestID      string     `gorm:"not null;index"`
	ClientID       string     `gorm:"not null;index"`
	UserID         uint       `gorm:"index"`
	Status         int        `gorm:"not null;default:0"`
	Scopes         string     `gorm:"not null"` // JSON []string
	SessionData    string     `gorm:"not null"`
	ExpiresAt      time.Time  `gorm:"not null;index"`
	LastCheckedAt  *time.Time
	CreatedAt      time.Time
}
