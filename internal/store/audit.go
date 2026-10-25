package store

import (
	"encoding/json"
	"time"
)

// AuditActor is who acted and the role their authority over the target came from.
type AuditActor struct {
	UserID  uint
	TokenID *uint
	Role    string // "admin" or "lead"
}

// Change is the audited state of the target before and after the mutation.
type Change struct {
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

// RecordAudit appends one event. Marshalling happens here so no caller can
// forget it or store something that is not JSON.
func (s *Store) RecordAudit(at time.Time, actor AuditActor, action, targetType string, targetID uint, c Change) error {
	if c.Before == nil {
		c.Before = map[string]any{}
	}
	if c.After == nil {
		c.After = map[string]any{}
	}
	changes, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.db.Create(&AuditEvent{
		At:           at,
		ActorUserID:  actor.UserID,
		ActorTokenID: actor.TokenID,
		ActorRole:    actor.Role,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		Changes:      string(changes),
	}).Error
}

// AuditFilter narrows AuditEvents; a zero field means any.
type AuditFilter struct {
	From, To    time.Time // From inclusive, To exclusive
	ActorUserID uint
	Action      string
	TargetType  string
	TargetID    uint
	Limit       int // <=0 means 200, capped at 1000
}

const (
	auditDefaultLimit = 200
	auditMaxLimit     = 1000
)

// AuditEvents returns the events matching f, newest first.
func (s *Store) AuditEvents(f AuditFilter) ([]AuditEvent, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = auditDefaultLimit
	}
	if limit > auditMaxLimit {
		limit = auditMaxLimit
	}
	q := s.db
	if !f.From.IsZero() {
		q = q.Where("at >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("at < ?", f.To)
	}
	if f.ActorUserID != 0 {
		q = q.Where("actor_user_id = ?", f.ActorUserID)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != 0 {
		q = q.Where("target_id = ?", f.TargetID)
	}
	out := []AuditEvent{}
	err := q.Order("at DESC, id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// AuditFields is the audited view of u; the password hash is never part of it.
func (u *User) AuditFields() map[string]any {
	return map[string]any{
		"id":                   u.ID,
		"email":                u.Email,
		"name":                 u.Name,
		"timezone":             u.Timezone,
		"is_admin":             u.IsAdmin,
		"active":               u.Active,
		"must_change_password": u.MustChangePassword,
		"created_at":           u.CreatedAt,
		"updated_at":           u.UpdatedAt,
	}
}

// AuditFields is the audited view of t.
func (t *Team) AuditFields() map[string]any {
	return map[string]any{
		"id":          t.ID,
		"name":        t.Name,
		"created_at":  t.CreatedAt,
		"updated_at":  t.UpdatedAt,
		"archived_at": t.ArchivedAt,
	}
}

// AuditFields is the audited view of p.
func (p *Project) AuditFields() map[string]any {
	return map[string]any{
		"id":          p.ID,
		"team_id":     p.TeamID,
		"name":        p.Name,
		"color":       p.Color,
		"created_at":  p.CreatedAt,
		"updated_at":  p.UpdatedAt,
		"archived_at": p.ArchivedAt,
	}
}

// AuditFields is the audited view of e; a running or deleted entry shows as nil.
func (e *Entry) AuditFields() map[string]any {
	return map[string]any{
		"id":         e.ID,
		"user_id":    e.UserID,
		"project_id": e.ProjectID,
		"start_at":   e.StartAt,
		"end_at":     e.EndAt,
		"timezone":   e.Timezone,
		"note":       e.Note,
		"created_at": e.CreatedAt,
		"updated_at": e.UpdatedAt,
		"deleted_at": e.DeletedAt,
	}
}
