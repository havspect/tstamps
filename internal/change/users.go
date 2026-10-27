package change

import (
	"context"
	"errors"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// Users manages mutations to user accounts.
type Users struct {
	store *store.Store
	clock clock.Clock
}

// UserPatch holds optional fields for updating a user.
type UserPatch struct {
	Name     *string
	Timezone *string
	IsAdmin  *bool
	Active   *bool
}

// CreateUser inserts u and commits an audit event within a single transaction.
func (s *Users) Create(ctx context.Context, actor Actor, u *store.User) (*store.User, error) {
	now := s.clock.Now()
	err := s.store.InTx(func(tx *store.Store) error {
		if err := tx.CreateUser(u); err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "user.create", "user", u.ID,
			store.Change{After: u.AuditFields()})
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Update applies patch to user id, handles deactivation cascades (stopping any
// running entry and logging the cascade event), and records the audit event.
func (s *Users) Update(ctx context.Context, actor Actor, id uint, patch UserPatch) (*store.User, error) {
	now := s.clock.Now()
	var after *store.User

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.UserByID(id)
		if err != nil {
			return err
		}
		u := *before
		if patch.Name != nil {
			u.Name = *patch.Name
		}
		if patch.Timezone != nil {
			u.Timezone = *patch.Timezone
		}
		if patch.IsAdmin != nil {
			u.IsAdmin = *patch.IsAdmin
		}
		if patch.Active != nil {
			u.Active = *patch.Active
		}

		// Read running entry if deactivating to capture before/after for audit
		var running *store.Entry
		if before.Active && !u.Active {
			running, err = tx.RunningEntry(id)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}

		if err := tx.SaveUser(&u, now); err != nil {
			return err
		}

		after, err = tx.UserByID(id)
		if err != nil {
			return err
		}

		auditActor := actor.AuditActor()
		if err := tx.RecordAudit(now, auditActor, "user.update", "user", id,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()}); err != nil {
			return err
		}

		if running != nil {
			stopped, err := tx.EntryByID(running.ID)
			if err != nil {
				return err
			}
			if err := tx.RecordAudit(now, auditActor, "entry.stop", "entry", running.ID,
				store.Change{Before: running.AuditFields(), After: stopped.AuditFields()}); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return after, nil
}

// ResetPassword stores the new password hash and creates an audit event.
func (s *Users) ResetPassword(ctx context.Context, actor Actor, id uint, hash string) (*store.User, error) {
	now := s.clock.Now()
	var after *store.User

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.UserByID(id)
		if err != nil {
			return err
		}
		if err := tx.SetUserPassword(id, hash, true, 0, now); err != nil {
			return err
		}
		after, err = tx.UserByID(id)
		if err != nil {
			return err
		}
		return tx.RecordAudit(now, actor.AuditActor(), "user.password.reset", "user", id,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})

	if err != nil {
		return nil, err
	}
	return after, nil
}
