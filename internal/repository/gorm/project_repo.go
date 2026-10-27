package gorm

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/project"
)

// ProjectRepository implements project.Repository and project.TeamChecker using GORM.
type ProjectRepository struct {
	db *gorm.DB
}

// NewProjectRepository creates a new ProjectRepository.
func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

var _ project.Repository = (*ProjectRepository)(nil)
var _ project.TeamChecker = (*ProjectRepository)(nil)

func toProjectDomain(m *Project) *project.Project {
	if m == nil {
		return nil
	}
	return &project.Project{
		ID:          m.ID,
		WorkspaceID: m.WorkspaceID,
		TeamID:      m.TeamID,
		Name:        m.Name,
		Color:       m.Color,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		ArchivedAt:  m.ArchivedAt,
	}
}

func toProjectGORM(p *project.Project) *Project {
	if p == nil {
		return nil
	}
	return &Project{
		ID:          p.ID,
		WorkspaceID: p.WorkspaceID,
		TeamID:      p.TeamID,
		Name:        p.Name,
		Color:       p.Color,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		ArchivedAt:  p.ArchivedAt,
	}
}

// GetByID returns the project by id.
func (r *ProjectRepository) GetByID(ctx context.Context, id uint) (*project.Project, error) {
	db := DBFrom(ctx, r.db)
	var m Project
	if err := db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, project.ErrNotFound
		}
		return nil, err
	}
	return toProjectDomain(&m), nil
}

// List lists projects according to filter.
func (r *ProjectRepository) List(ctx context.Context, filter project.ListFilter) ([]project.Project, error) {
	db := DBFrom(ctx, r.db)
	q := db.Model(&Project{})
	if !filter.IncludeArchived {
		q = q.Where("archived_at IS NULL")
	}
	if filter.TeamID != 0 {
		q = q.Where("team_id = ?", filter.TeamID)
	}
	if len(filter.TeamIDs) > 0 {
		q = q.Where("team_id IN ?", filter.TeamIDs)
	}

	var rows []Project
	if err := q.Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]project.Project, 0, len(rows))
	for i := range rows {
		out = append(out, *toProjectDomain(&rows[i]))
	}
	return out, nil
}

// Create inserts a project into the workspace.
func (r *ProjectRepository) Create(ctx context.Context, p *project.Project) error {
	db := DBFrom(ctx, r.db)
	m := toProjectGORM(p)
	if err := db.Create(m).Error; err != nil {
		return err
	}
	p.ID = m.ID
	p.CreatedAt = m.CreatedAt
	p.UpdatedAt = m.UpdatedAt
	return nil
}

// Update updates name, color, team, archived_at, and updated_at.
func (r *ProjectRepository) Update(ctx context.Context, p *project.Project) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Project{}).Where("id = ?", p.ID).Select(
		"TeamID", "Name", "Color", "ArchivedAt", "UpdatedAt",
	).Updates(map[string]any{
		"team_id":     p.TeamID,
		"name":        p.Name,
		"color":       p.Color,
		"archived_at": p.ArchivedAt,
		"updated_at":  p.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return project.ErrNotFound
	}
	return nil
}

// Archive archives a project by ID.
func (r *ProjectRepository) Archive(ctx context.Context, id uint, at time.Time) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Project{}).Where("id = ? AND archived_at IS NULL", id).Updates(map[string]any{
		"archived_at": at,
		"updated_at":  at,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return project.ErrNotFound
	}
	return nil
}

// Restore unarchives a project by ID.
func (r *ProjectRepository) Restore(ctx context.Context, id uint) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Project{}).Where("id = ? AND archived_at IS NOT NULL", id).Updates(map[string]any{
		"archived_at": nil,
		"updated_at":  time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return project.ErrNotFound
	}
	return nil
}

// HasEntries reports whether any time entries exist booked against this project.
func (r *ProjectRepository) HasEntries(ctx context.Context, id uint) (bool, error) {
	db := DBFrom(ctx, r.db)
	var count int64
	if err := db.Model(&Entry{}).Where("project_id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsTeamActive reports whether teamID exists in the workspace and is not archived.
func (r *ProjectRepository) IsTeamActive(ctx context.Context, teamID uint) (bool, error) {
	db := DBFrom(ctx, r.db)
	var t Team
	err := db.Select("id", "archived_at").Where("id = ?", teamID).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if t.ArchivedAt != nil {
		return false, nil
	}
	return true, nil
}
