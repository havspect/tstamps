// Package audit provides domain models and services for tamper-evident workspace audit logging.
package audit

import (
	"context"
	"time"
)

// AuditEvent represents a single immutable audit log record.
type AuditEvent struct {
	ID            uint
	WorkspaceID   uint
	At            time.Time
	ActorUserID   uint
	ActorTokenID  *uint
	ActorClientID string
	ActorGrantID  string
	ActorRole     string
	Action        string
	TargetType    string
	TargetID      uint
	Changes       string
	CreatedAt     time.Time
}

// QueryFilter specifies criteria to narrow audit event queries.
type QueryFilter struct {
	From        time.Time
	To          time.Time
	ActorUserID uint
	Action      string
	TargetType  string
	TargetID    uint
	Limit       int
}

// Repository defines persistence operations for audit events.
type Repository interface {
	Record(ctx context.Context, e *AuditEvent) error
	Query(ctx context.Context, filter QueryFilter) ([]AuditEvent, error)
}
