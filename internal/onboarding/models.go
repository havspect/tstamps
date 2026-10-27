package onboarding

import "time"

// Workspace represents a tenant workspace domain model.
type Workspace struct {
	ID        uint
	Slug      string
	CreatedAt time.Time
}

// PendingSignup represents an unverified signup link token.
type PendingSignup struct {
	TokenHash string
	Email     string
	CreatedAt time.Time
}

// SignupResult represents the outcome of completing a workspace signup.
type SignupResult struct {
	WorkspaceSlug string
	Email         string
	Name          string
	Token         string
	UserID        uint
}
