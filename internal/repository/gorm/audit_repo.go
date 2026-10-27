package gorm

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/project"
)

// AuditRepository implements audit.Repository using GORM.
type AuditRepository struct {
	db *gorm.DB
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

var _ audit.Repository = (*AuditRepository)(nil)

func toAuditDomain(m *AuditLog) audit.AuditEvent {
	return audit.AuditEvent{
		ID:            m.ID,
		WorkspaceID:   m.WorkspaceID,
		At:            m.At,
		ActorUserID:   m.ActorUserID,
		ActorTokenID:  m.ActorTokenID,
		ActorClientID: m.ActorClientID,
		ActorGrantID:  m.ActorGrantID,
		ActorRole:     m.ActorRole,
		Action:        m.Action,
		TargetType:    m.TargetType,
		TargetID:      m.TargetID,
		Changes:       m.Changes,
		CreatedAt:     m.CreatedAt,
	}
}

// Record persists an audit event.
func (r *AuditRepository) Record(ctx context.Context, e *audit.AuditEvent) error {
	db := DBFrom(ctx, r.db)
	m := &AuditLog{
		WorkspaceID:   e.WorkspaceID,
		At:            e.At,
		ActorUserID:   e.ActorUserID,
		ActorTokenID:  e.ActorTokenID,
		ActorClientID: e.ActorClientID,
		ActorGrantID:  e.ActorGrantID,
		ActorRole:     e.ActorRole,
		Action:        e.Action,
		TargetType:    e.TargetType,
		TargetID:      e.TargetID,
		Changes:       e.Changes,
		CreatedAt:     e.CreatedAt,
	}
	if err := db.Create(m).Error; err != nil {
		return err
	}
	e.ID = m.ID
	return nil
}

// Query returns audit events matching the filter.
func (r *AuditRepository) Query(ctx context.Context, filter audit.QueryFilter) ([]audit.AuditEvent, error) {
	db := DBFrom(ctx, r.db)
	q := db.Model(&AuditLog{})
	if !filter.From.IsZero() {
		q = q.Where("at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("at < ?", filter.To)
	}
	if filter.ActorUserID != 0 {
		q = q.Where("actor_user_id = ?", filter.ActorUserID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.TargetType != "" {
		q = q.Where("target_type = ?", filter.TargetType)
	}
	if filter.TargetID != 0 {
		q = q.Where("target_id = ?", filter.TargetID)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	var rows []AuditLog
	if err := q.Order("at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]audit.AuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, toAuditDomain(&rows[i]))
	}
	return out, nil
}

// AuditRecorderAdapter adapts AuditRepository to entry.AuditRecorder and project.AuditRecorder.
type AuditRecorderAdapter struct {
	repo *AuditRepository
}

// NewAuditRecorderAdapter creates an AuditRecorderAdapter.
func NewAuditRecorderAdapter(repo *AuditRepository) *AuditRecorderAdapter {
	return &AuditRecorderAdapter{repo: repo}
}

var _ entry.AuditRecorder = (*AuditRecorderAdapter)(nil)
var _ project.AuditRecorder = (*AuditRecorderAdapter)(nil)

// Record records an action with structured changes.
func (a *AuditRecorderAdapter) Record(ctx context.Context, action string, targetType string, targetID uint, changes any) error {
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return err
	}

	var actorUserID uint
	var actorRole string
	var actorClientID string
	var actorTokenID *uint

	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorUserID = p.UserID
		actorClientID = p.ClientID
		if len(p.Roles) > 0 {
			actorRole = p.Roles[0]
		}
	}

	evt := &audit.AuditEvent{
		At:            time.Now(),
		ActorUserID:   actorUserID,
		ActorTokenID:  actorTokenID,
		ActorClientID: actorClientID,
		ActorRole:     actorRole,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		Changes:       string(changesJSON),
		CreatedAt:     time.Now(),
	}
	return a.repo.Record(ctx, evt)
}
