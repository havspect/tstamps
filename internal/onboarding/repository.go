package onboarding

import "context"

// Repository defines the persistence interface required for onboarding operations.
type Repository interface {
	CreateWorkspace(ctx context.Context, slug string) (*Workspace, error)
	GetWorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error)
	ListWorkspaces(ctx context.Context) ([]Workspace, error)
	CreatePendingSignup(ctx context.Context, email, tokenHash string) error
	GetPendingSignup(ctx context.Context, tokenHash string) (*PendingSignup, error)
	DeletePendingSignup(ctx context.Context, tokenHash string) error
	CreateInitialAdmin(ctx context.Context, workspaceID uint, email, name, passwordHash, timezone string) (uint, error)
}
