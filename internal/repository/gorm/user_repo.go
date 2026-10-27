package gorm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/user"
	"gorm.io/gorm"
)

// UserRepository provides GORM persistence for users, sessions, and tokens.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository returns a new UserRepository backed by db.
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

var _ user.Repository = (*UserRepository)(nil)

func toDomainUser(u *User) *user.User {
	if u == nil {
		return nil
	}
	return &user.User{
		ID:                 u.ID,
		WorkspaceID:        u.WorkspaceID,
		Email:              u.Email,
		Name:               u.Name,
		PasswordHash:       u.PasswordHash,
		Timezone:           u.Timezone,
		IsAdmin:            u.IsAdmin,
		Active:             u.Active,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}
}

func toDomainAPIToken(t *APIToken) *user.APIToken {
	if t == nil {
		return nil
	}
	var expiresAt *time.Time
	if !t.ExpiresAt.IsZero() {
		exp := t.ExpiresAt
		expiresAt = &exp
	}
	return &user.APIToken{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		UserID:      t.UserID,
		Name:        t.Name,
		TokenHash:   t.TokenHash,
		ExpiresAt:   expiresAt,
		LastUsedAt:  t.LastUsedAt,
		RevokedAt:   t.RevokedAt,
		CreatedAt:   t.CreatedAt,
	}
}

// GetByID returns the user with id, or user.ErrNotFound.
func (r *UserRepository) GetByID(ctx context.Context, id uint) (*user.User, error) {
	var row User
	if err := DBFrom(ctx, r.db).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return toDomainUser(&row), nil
}

// GetByEmail returns the user with the given email address, or user.ErrNotFound.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	var row User
	if err := DBFrom(ctx, r.db).Where("email = ?", strings.ToLower(email)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return toDomainUser(&row), nil
}

// List returns all users in the workspace ordered by email.
func (r *UserRepository) List(ctx context.Context) ([]user.User, error) {
	var rows []User
	if err := DBFrom(ctx, r.db).Order("email ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]user.User, len(rows))
	for i, row := range rows {
		out[i] = *toDomainUser(&row)
	}
	return out, nil
}

// Create inserts a new user in the workspace.
func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	row := User{
		WorkspaceID:        u.WorkspaceID,
		Email:              strings.ToLower(u.Email),
		Name:               u.Name,
		PasswordHash:       u.PasswordHash,
		Timezone:           u.Timezone,
		IsAdmin:            u.IsAdmin,
		Active:             u.Active,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}

	if err := DBFrom(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}

	u.ID = row.ID
	u.WorkspaceID = row.WorkspaceID
	return nil
}

// Update updates a user's mutable attributes.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	res := DBFrom(ctx, r.db).Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"email":                strings.ToLower(u.Email),
		"name":                 u.Name,
		"timezone":             u.Timezone,
		"is_admin":             u.IsAdmin,
		"active":               u.Active,
		"password_hash":        u.PasswordHash,
		"must_change_password": u.MustChangePassword,
		"updated_at":           u.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return user.ErrNotFound
	}
	return nil
}

// CountActiveAdmins returns the number of active administrator users in the workspace.
func (r *UserRepository) CountActiveAdmins(ctx context.Context) (int, error) {
	var count int64
	err := DBFrom(ctx, r.db).Model(&User{}).
		Where("is_admin = ? AND active = ?", true, true).
		Count(&count).Error
	return int(count), err
}

// SaveSession saves a login session.
func (r *UserRepository) SaveSession(ctx context.Context, s *user.Session) error {
	row := Session{
		WorkspaceID: s.WorkspaceID,
		UserID:      s.UserID,
		TokenHash:   s.TokenHash,
		ExpiresAt:   s.ExpiresAt,
		CreatedAt:   s.CreatedAt,
	}

	if err := DBFrom(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}

	s.ID = row.ID
	s.WorkspaceID = row.WorkspaceID
	return nil
}

// GetSessionByTokenHash retrieves an active session by token hash.
func (r *UserRepository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*user.Session, error) {
	var row Session
	if err := DBFrom(ctx, r.db).Where("token_hash = ?", tokenHash).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return &user.Session{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		UserID:      row.UserID,
		TokenHash:   row.TokenHash,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// DeleteSession deletes a login session by its token hash.
func (r *UserRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	return DBFrom(ctx, r.db).Where("token_hash = ?", tokenHash).Delete(&Session{}).Error
}

// DeleteUserSessionsExcept deletes all sessions for a user except the specified keepTokenHash.
func (r *UserRepository) DeleteUserSessionsExcept(ctx context.Context, userID uint, keepTokenHash string) error {
	q := DBFrom(ctx, r.db).Where("user_id = ?", userID)
	if keepTokenHash != "" {
		q = q.Where("token_hash <> ?", keepTokenHash)
	}
	return q.Delete(&Session{}).Error
}

// CreateAPIToken inserts a new personal API access token.
func (r *UserRepository) CreateAPIToken(ctx context.Context, t *user.APIToken) error {
	var expiresAt time.Time
	if t.ExpiresAt != nil {
		expiresAt = *t.ExpiresAt
	} else {
		// Default far future expiration if not provided
		expiresAt = time.Now().AddDate(10, 0, 0)
	}

	row := APIToken{
		WorkspaceID: t.WorkspaceID,
		UserID:      t.UserID,
		Name:        t.Name,
		TokenHash:   t.TokenHash,
		ExpiresAt:   expiresAt,
		LastUsedAt:  t.LastUsedAt,
		RevokedAt:   t.RevokedAt,
		CreatedAt:   t.CreatedAt,
	}

	if err := DBFrom(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}

	t.ID = row.ID
	t.WorkspaceID = row.WorkspaceID
	return nil
}

// GetAPITokenByTokenHash retrieves an active personal API token by its token hash.
func (r *UserRepository) GetAPITokenByTokenHash(ctx context.Context, tokenHash string) (*user.APIToken, error) {
	var row APIToken
	if err := DBFrom(ctx, r.db).Where("token_hash = ? AND revoked_at IS NULL", tokenHash).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return toDomainAPIToken(&row), nil
}

// TouchAPIToken updates last_used_at on the specified token.
func (r *UserRepository) TouchAPIToken(ctx context.Context, id uint, at time.Time) error {
	return DBFrom(ctx, r.db).Model(&APIToken{}).Where("id = ?", id).Update("last_used_at", at).Error
}

// RevokeAPIToken marks an API access token revoked.
func (r *UserRepository) RevokeAPIToken(ctx context.Context, id uint, userID uint) error {
	now := time.Now()
	res := DBFrom(ctx, r.db).Model(&APIToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return user.ErrNotFound
	}
	return nil
}

// ListAPITokens returns all API tokens for the given user, ordered by ID.
func (r *UserRepository) ListAPITokens(ctx context.Context, userID uint) ([]user.APIToken, error) {
	var rows []APIToken
	if err := DBFrom(ctx, r.db).Where("user_id = ?", userID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]user.APIToken, len(rows))
	for i, row := range rows {
		out[i] = *toDomainAPIToken(&row)
	}
	return out, nil
}

// RevokeAllOAuthTokens invalidates all active OAuth tokens, codes, and device authorizations for userID.
func (r *UserRepository) RevokeAllOAuthTokens(ctx context.Context, userID uint) error {
	db := DBFrom(ctx, r.db)
	now := time.Now()

	if err := db.Model(&OAuthAccessToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error; err != nil {
		return err
	}

	if err := db.Model(&OAuthRefreshToken{}).
		Where("user_id = ? AND active = ? AND revoked_at IS NULL", userID, true).
		Updates(map[string]any{"active": false, "revoked_at": now}).Error; err != nil {
		return err
	}

	if err := db.Model(&OAuthCode{}).
		Where("user_id = ? AND used = ?", userID, false).
		Update("used", true).Error; err != nil {
		return err
	}

	if err := db.Model(&OAuthDeviceCode{}).
		Where("user_id = ?", userID).
		Update("status", 2).Error; err != nil {
		return err
	}

	return nil
}
