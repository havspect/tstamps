// Package change coordinates workspace mutations across aggregates. It owns the
// transaction boundary, domain preconditions, cascade effects and audit event
// generation, so HTTP handlers and tools only translate input and output.
package change

import (
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

// Actor is the authenticated principal carrying out the change, along with
// their role and token provenance for audit records.
type Actor struct {
	UserID   uint
	TokenID  *uint
	ClientID string
	GrantID  string
	Role     string // "admin", "lead", or empty for self-service
}

func (a Actor) AuditActor() store.AuditActor {
	return store.AuditActor{
		UserID:   a.UserID,
		TokenID:  a.TokenID,
		ClientID: a.ClientID,
		GrantID:  a.GrantID,
		Role:     a.Role,
	}
}

// Module provides the change services across domain aggregates.
type Module struct {
	Users    *Users
	Teams    *Teams
	Projects *Projects
	Entries  *Entries
	store    *store.Store
	clock    clock.Clock
}

// New returns a new change Module backed by store and clock.
func New(st *store.Store, clk clock.Clock) *Module {
	m := &Module{store: st, clock: clk}
	m.Users = &Users{store: st, clock: clk}
	m.Teams = &Teams{store: st, clock: clk}
	m.Projects = &Projects{store: st, clock: clk}
	m.Entries = &Entries{store: st, clock: clk}
	return m
}
