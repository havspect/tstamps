package store

import (
	"errors"
	"time"
)

// CreateProject inserts a project into this workspace.
func (s *Store) CreateProject(p *Project) error {
	p.WorkspaceID = s.workspaceID
	return s.db.Create(p).Error
}

// ProjectByID returns the project with id in this workspace, archived or not; an
// unknown id returns ErrNotFound.
func (s *Store) ProjectByID(id uint) (*Project, error) {
	var p Project
	if err := s.scoped().First(&p, id).Error; err != nil {
		return nil, translate(err)
	}
	return &p, nil
}

// Projects lists this workspace's projects ordered by name; archived ones only
// when includeArchived.
func (s *Store) Projects(includeArchived bool) ([]Project, error) {
	out := []Project{}
	q := s.scoped().Order("name")
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	err := q.Find(&out).Error
	return out, err
}

// UpdateProject writes p's name, color and archive state; it never inserts.
// Moving a project to another team is MoveProject, which can refuse.
func (s *Store) UpdateProject(p *Project) error {
	return s.scoped().Model(p).Select("Name", "Color", "ArchivedAt").Updates(p).Error
}

// ErrProjectHasEntries is returned by MoveProject while any entry, deleted ones
// included, is booked on the project: a restored entry must not land in a
// different team.
var ErrProjectHasEntries = errors.New("project has entries")

// MoveProject changes a project's team in one statement that refuses while the
// project has entries, so an entry cannot slip in between a check and a write.
func (s *Store) MoveProject(id, teamID uint, at time.Time) error {
	res := s.scoped().Model(&Project{}).
		Where("id = ? AND NOT EXISTS (SELECT 1 FROM entries WHERE project_id = ?)", id, id).
		UpdateColumns(map[string]any{"team_id": teamID, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrProjectHasEntries
	}
	return nil
}

// TeamAcceptsProjects reports whether teamID exists in this workspace and is not
// archived, the condition for creating a project there or moving one into it.
func (s *Store) TeamAcceptsProjects(teamID uint) (bool, error) {
	var n int64
	err := s.scoped().Model(&Team{}).Where("id = ? AND archived_at IS NULL", teamID).Count(&n).Error
	return n > 0, err
}
