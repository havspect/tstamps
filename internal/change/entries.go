package change

import (
	"context"
	"time"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/store"
)

// Entries manages mutations and restores of time entries.
type Entries struct {
	store *store.Store
	clock clock.Clock
}

// Restore brings back a soft-deleted entry and records an audit event in one transaction.
func (s *Entries) Restore(ctx context.Context, actor Actor, id uint) (*store.Entry, error) {
	var restored *store.Entry
	var at time.Time

	err := s.store.InTx(func(tx *store.Store) error {
		before, err := tx.EntryByID(id)
		if err != nil {
			return err
		}
		restored, err = entry.New(tx, s.clock).Restore(id)
		if err != nil {
			return err
		}
		at = restored.UpdatedAt
		return tx.RecordAudit(at, actor.AuditActor(), "entry.restore", "entry", id,
			store.Change{Before: before.AuditFields(), After: restored.AuditFields()})
	})

	if err != nil {
		return nil, err
	}
	return restored, nil
}
