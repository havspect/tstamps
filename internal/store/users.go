package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrLastAdmin is returned when an update would leave the workspace without an active admin.
var ErrLastAdmin = errors.New("last admin")

// ListUsers returns every user in this workspace, deactivated ones included,
// ordered by email.
func (s *Store) ListUsers() ([]User, error) {
	var users []User
	err := s.scoped().Order("email").Find(&users).Error
	return users, err
}

// SaveUser writes the admin-editable fields of u and refuses to leave no active
// admin. Deactivating also stops the running entry at at, in the same
// transaction, so a user who can no longer log in is not left with a timer
// nobody can reach.
func (s *Store) SaveUser(u *User, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		ws := tx.Where("workspace_id = ?", s.workspaceID)
		var current User
		if err := ws.First(&current, u.ID).Error; err != nil {
			return translate(err)
		}
		if current.IsAdmin && current.Active && (!u.IsAdmin || !u.Active) {
			// Count and write in one transaction so two concurrent demotions cannot both pass.
			var admins int64
			err := tx.Model(&User{}).Where("workspace_id = ? AND is_admin AND active", s.workspaceID).Count(&admins).Error
			if err != nil {
				return err
			}
			if admins <= 1 {
				return ErrLastAdmin
			}
		}
		if current.Active && !u.Active {
			if err := tx.Model(&Entry{}).
				Where("workspace_id = ? AND user_id = ? AND end_at IS NULL AND deleted_at IS NULL", s.workspaceID, u.ID).
				UpdateColumns(map[string]any{"end_at": at, "note": DeactivationNote, "updated_at": at}).Error; err != nil {
				return err
			}
			if err := tx.Model(&OAuthAccessToken{}).
				Where("workspace_id = ? AND user_id = ? AND revoked_at IS NULL", s.workspaceID, u.ID).
				Update("revoked_at", at).Error; err != nil {
				return err
			}
			if err := tx.Model(&OAuthRefreshToken{}).
				Where("workspace_id = ? AND user_id = ? AND active = ? AND revoked_at IS NULL", s.workspaceID, u.ID, true).
				Updates(map[string]any{"active": false, "revoked_at": at}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&User{}).Where("workspace_id = ? AND id = ?", s.workspaceID, u.ID).Updates(map[string]any{
			"name":     u.Name,
			"timezone": u.Timezone,
			"is_admin": u.IsAdmin,
			"active":   u.Active,
		}).Error
	})
}

// SetUserPassword stores hash and sets must_change_password in one transaction with
// ending the user's other logins: every session except keepSessionID (0 keeps none)
// and every token that is not revoked yet. An unknown id returns ErrNotFound.
func (s *Store) SetUserPassword(id uint, hash string, mustChange bool, keepSessionID uint, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&User{}).Where("workspace_id = ? AND id = ?", s.workspaceID, id).
			Updates(map[string]any{"password_hash": hash, "must_change_password": mustChange})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Where("workspace_id = ? AND user_id = ? AND id <> ?", s.workspaceID, id, keepSessionID).
			Delete(&Session{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&APIToken{}).
			Where("workspace_id = ? AND user_id = ? AND revoked_at IS NULL", s.workspaceID, id).
			Update("revoked_at", at).Error; err != nil {
			return err
		}
		if err := tx.Model(&OAuthAccessToken{}).
			Where("workspace_id = ? AND user_id = ? AND revoked_at IS NULL", s.workspaceID, id).
			Update("revoked_at", at).Error; err != nil {
			return err
		}
		if err := tx.Model(&OAuthRefreshToken{}).
			Where("workspace_id = ? AND user_id = ? AND active = ? AND revoked_at IS NULL", s.workspaceID, id, true).
			Updates(map[string]any{"active": false, "revoked_at": at}).Error; err != nil {
			return err
		}
		if err := tx.Model(&OAuthCode{}).
			Where("workspace_id = ? AND user_id = ? AND used = ?", s.workspaceID, id, false).
			Update("used", true).Error; err != nil {
			return err
		}
		return tx.Model(&OAuthDeviceCode{}).
			Where("workspace_id = ? AND user_id = ?", s.workspaceID, id).
			Update("status", 2).Error // 2 = denied/invalidated status for device codes
	})
}
