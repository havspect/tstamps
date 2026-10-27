package gorm

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/entry"
)

// EntryRepository implements entry.Repository and entry.ProjectChecker using GORM.
type EntryRepository struct {
	db *gorm.DB
}

// NewEntryRepository creates a new EntryRepository.
func NewEntryRepository(db *gorm.DB) *EntryRepository {
	return &EntryRepository{db: db}
}

var _ entry.Repository = (*EntryRepository)(nil)
var _ entry.ProjectChecker = (*EntryRepository)(nil)

func toEntryDomain(m *Entry) *entry.Entry {
	if m == nil {
		return nil
	}
	return &entry.Entry{
		ID:          m.ID,
		WorkspaceID: m.WorkspaceID,
		UserID:      m.UserID,
		ProjectID:   m.ProjectID,
		StartAt:     m.StartAt,
		EndAt:       m.EndAt,
		Timezone:    m.Timezone,
		Note:        m.Note,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		DeletedAt:   m.DeletedAt,
	}
}

func toEntryGORM(e *entry.Entry) *Entry {
	if e == nil {
		return nil
	}
	return &Entry{
		ID:          e.ID,
		WorkspaceID: e.WorkspaceID,
		UserID:      e.UserID,
		ProjectID:   e.ProjectID,
		StartAt:     e.StartAt,
		EndAt:       e.EndAt,
		Timezone:    e.Timezone,
		Note:        e.Note,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		DeletedAt:   e.DeletedAt,
	}
}

// GetByID returns the entry by its primary key ID.
func (r *EntryRepository) GetByID(ctx context.Context, id uint) (*entry.Entry, error) {
	db := DBFrom(ctx, r.db)
	var m Entry
	if err := db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entry.ErrNotFound
		}
		return nil, err
	}
	return toEntryDomain(&m), nil
}

// GetRunning returns the active running entry for userID, or entry.ErrNotFound if none.
func (r *EntryRepository) GetRunning(ctx context.Context, userID uint) (*entry.Entry, error) {
	db := DBFrom(ctx, r.db)
	var m Entry
	err := db.Where("user_id = ? AND end_at IS NULL AND deleted_at IS NULL", userID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, entry.ErrNotFound
		}
		return nil, err
	}
	return toEntryDomain(&m), nil
}

// List returns entries matching the given filter.
func (r *EntryRepository) List(ctx context.Context, filter entry.ListFilter) ([]entry.Entry, error) {
	db := DBFrom(ctx, r.db)
	q := db.Model(&Entry{})
	if !filter.IncludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
	if filter.UserID != 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.ProjectID != 0 {
		q = q.Where("project_id = ?", filter.ProjectID)
	}
	if len(filter.ProjectIDs) > 0 {
		q = q.Where("project_id IN ?", filter.ProjectIDs)
	}
	if filter.From != nil {
		q = q.Where("end_at IS NULL OR end_at > ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("start_at < ?", *filter.To)
	}

	var rows []Entry
	if err := q.Order("start_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]entry.Entry, 0, len(rows))
	for i := range rows {
		out = append(out, *toEntryDomain(&rows[i]))
	}
	return out, nil
}

// Create persists a new entry.
func (r *EntryRepository) Create(ctx context.Context, e *entry.Entry) error {
	db := DBFrom(ctx, r.db)
	m := toEntryGORM(e)
	err := db.Create(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return entry.ErrRunningExists
		}
		return err
	}
	e.ID = m.ID
	e.CreatedAt = m.CreatedAt
	e.UpdatedAt = m.UpdatedAt
	return nil
}

// Update mutates an existing entry.
func (r *EntryRepository) Update(ctx context.Context, e *entry.Entry) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Entry{}).Where("id = ?", e.ID).Select(
		"ProjectID", "StartAt", "EndAt", "Timezone", "Note", "UpdatedAt",
	).Updates(map[string]any{
		"project_id": e.ProjectID,
		"start_at":   e.StartAt,
		"end_at":     e.EndAt,
		"timezone":   e.Timezone,
		"note":       e.Note,
		"updated_at": e.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return entry.ErrNotFound
	}
	return nil
}

// SoftDelete marks an entry as deleted.
func (r *EntryRepository) SoftDelete(ctx context.Context, id uint, deletedAt time.Time) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Entry{}).Where("id = ? AND deleted_at IS NULL", id).Updates(map[string]any{
		"deleted_at": deletedAt,
		"updated_at": deletedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return entry.ErrNotFound
	}
	return nil
}

// Restore restores a soft-deleted entry.
func (r *EntryRepository) Restore(ctx context.Context, id uint) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Entry{}).Where("id = ?", id).Updates(map[string]any{
		"deleted_at": nil,
		"updated_at": time.Now(),
	})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return entry.ErrRunningExists
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return entry.ErrNotFound
	}
	return nil
}

// IsProjectLoggable reports whether the project exists and is not archived.
func (r *EntryRepository) IsProjectLoggable(ctx context.Context, projectID uint) (bool, error) {
	db := DBFrom(ctx, r.db)
	var p Project
	err := db.Select("id", "archived_at").Where("id = ?", projectID).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if p.ArchivedAt != nil {
		return false, nil
	}
	return true, nil
}

// StopRunningTimer stops a running timer for the specified user (satisfies user.TimerStopper).
func (r *EntryRepository) StopRunningTimer(ctx context.Context, userID uint, note string) error {
	db := DBFrom(ctx, r.db)
	now := time.Now()
	updates := map[string]any{
		"end_at":     now,
		"updated_at": now,
	}
	if note != "" {
		updates["note"] = note
	}
	res := db.Model(&Entry{}).
		Where("user_id = ? AND end_at IS NULL AND deleted_at IS NULL", userID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return entry.ErrNotFound
	}
	return nil
}

