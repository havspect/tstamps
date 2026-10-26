package store

import (
	"time"

	"gorm.io/gorm"
)

// CreatePendingSignup stores an emailed link's token hash and address.
func (r *Registry) CreatePendingSignup(tokenHash, email string, at time.Time) error {
	return r.db.Create(&PendingSignup{TokenHash: tokenHash, Email: email, CreatedAt: at}).Error
}

// PendingSignupEmail returns the address a token was sent to, or ErrNotFound.
func (r *Registry) PendingSignupEmail(tokenHash string) (string, error) {
	var p PendingSignup
	if err := r.db.First(&p, "token_hash = ?", tokenHash).Error; err != nil {
		return "", translate(err)
	}
	return p.Email, nil
}

// ClaimPendingSignup deletes the pending row and returns its address, so exactly
// one submit can use a link.
func (r *Registry) ClaimPendingSignup(tokenHash string) (string, error) {
	var email string
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var p PendingSignup
		if err := tx.First(&p, "token_hash = ?", tokenHash).Error; err != nil {
			return translate(err)
		}
		if err := tx.Delete(&p).Error; err != nil {
			return err
		}
		email = p.Email
		return nil
	})
	return email, err
}

// DeletePendingSignup removes a link that never reached the person.
func (r *Registry) DeletePendingSignup(tokenHash string) error {
	return r.db.Delete(&PendingSignup{}, "token_hash = ?", tokenHash).Error
}
