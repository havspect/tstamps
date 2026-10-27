package gorm

import (
	"context"
	"errors"
	"time"

	gormdriver "gorm.io/gorm"

	"github.com/havspect/tstamps/internal/onboarding"
)

var _ onboarding.Repository = (*OnboardingRepository)(nil)

// OnboardingRepository implements onboarding.Repository over GORM.
type OnboardingRepository struct {
	db *gormdriver.DB
}

// NewOnboardingRepository constructs an OnboardingRepository with the provided DB handle.
func NewOnboardingRepository(db *gormdriver.DB) *OnboardingRepository {
	return &OnboardingRepository{db: db}
}

// CreateWorkspace inserts a new workspace record.
func (r *OnboardingRepository) CreateWorkspace(ctx context.Context, slug string) (*onboarding.Workspace, error) {
	db := DBFrom(ctx, r.db)
	ws := &Workspace{
		Slug:      slug,
		CreatedAt: time.Now(),
	}
	if err := db.Create(ws).Error; err != nil {
		if errors.Is(err, gormdriver.ErrDuplicatedKey) {
			return nil, onboarding.ErrSlugTaken
		}
		return nil, err
	}
	return &onboarding.Workspace{
		ID:        ws.ID,
		Slug:      ws.Slug,
		CreatedAt: ws.CreatedAt,
	}, nil
}

// GetWorkspaceBySlug retrieves a workspace by its unique slug.
func (r *OnboardingRepository) GetWorkspaceBySlug(ctx context.Context, slug string) (*onboarding.Workspace, error) {
	var ws Workspace
	db := DBFrom(ctx, r.db)
	if err := db.Where("slug = ?", slug).First(&ws).Error; err != nil {
		if errors.Is(err, gormdriver.ErrRecordNotFound) {
			return nil, onboarding.ErrNotFound
		}
		return nil, err
	}
	return &onboarding.Workspace{
		ID:        ws.ID,
		Slug:      ws.Slug,
		CreatedAt: ws.CreatedAt,
	}, nil
}

// ListWorkspaces returns all workspaces ordered by ID ascending.
func (r *OnboardingRepository) ListWorkspaces(ctx context.Context) ([]onboarding.Workspace, error) {
	var list []Workspace
	db := DBFrom(ctx, r.db)
	if err := db.Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	res := make([]onboarding.Workspace, len(list))
	for i, w := range list {
		res[i] = onboarding.Workspace{
			ID:        w.ID,
			Slug:      w.Slug,
			CreatedAt: w.CreatedAt,
		}
	}
	return res, nil
}

// CreatePendingSignup records an unverified signup link token.
func (r *OnboardingRepository) CreatePendingSignup(ctx context.Context, email, tokenHash string) error {
	db := DBFrom(ctx, r.db)
	p := &PendingSignup{
		TokenHash: tokenHash,
		Email:     email,
		CreatedAt: time.Now(),
	}
	return db.Create(p).Error
}

// GetPendingSignup retrieves a pending signup by token hash.
func (r *OnboardingRepository) GetPendingSignup(ctx context.Context, tokenHash string) (*onboarding.PendingSignup, error) {
	var p PendingSignup
	db := DBFrom(ctx, r.db)
	if err := db.Where("token_hash = ?", tokenHash).First(&p).Error; err != nil {
		if errors.Is(err, gormdriver.ErrRecordNotFound) {
			return nil, onboarding.ErrNotFound
		}
		return nil, err
	}
	return &onboarding.PendingSignup{
		TokenHash: p.TokenHash,
		Email:     p.Email,
		CreatedAt: p.CreatedAt,
	}, nil
}

// DeletePendingSignup removes a claimed or cancelled pending signup.
func (r *OnboardingRepository) DeletePendingSignup(ctx context.Context, tokenHash string) error {
	db := DBFrom(ctx, r.db)
	return db.Where("token_hash = ?", tokenHash).Delete(&PendingSignup{}).Error
}

// CreateInitialAdmin creates the initial administrative user for the given workspace.
func (r *OnboardingRepository) CreateInitialAdmin(ctx context.Context, workspaceID uint, email, name, passwordHash, timezone string) (uint, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	userCtx := WithWorkspaceID(ctx, workspaceID)
	u := &User{
		WorkspaceID:        workspaceID,
		Email:              email,
		Name:               name,
		PasswordHash:       passwordHash,
		Timezone:           timezone,
		IsAdmin:            true,
		Active:             true,
		MustChangePassword: false,
	}
	db := DBFrom(userCtx, r.db)
	if err := db.Create(u).Error; err != nil {
		return 0, err
	}
	return u.ID, nil
}
