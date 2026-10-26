package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrEntryRunning is returned when a write would leave a user with a second
// running entry; the partial unique index is the authority, this names it.
var ErrEntryRunning = errors.New("user already has a running entry")

// DeactivationNote is the note deactivating a user leaves on the entry it stops.
const DeactivationNote = "stopped on deactivation"

// Scope narrows an entry query to what the caller may read. The zero value
// matches nothing, so a handler that forgets to set one returns no rows.
type Scope struct {
	// All grants every entry in the workspace (admin).
	All bool
	// UserID includes entries logged by this user.
	UserID uint
	// ProjectIDs includes entries on these projects.
	ProjectIDs []uint
	// IncludeDeleted also returns soft-deleted entries (admin listings).
	IncludeDeleted bool
}

// CreateEntry inserts e into this workspace. A second running entry for the same
// user returns ErrEntryRunning, whichever of the two writers loses the race.
func (s *Store) CreateEntry(e *Entry) error {
	e.WorkspaceID = s.workspaceID
	err := s.db.Create(e).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrEntryRunning
	}
	return err
}

// EntryByID returns the entry with id in this workspace, deleted ones included;
// an unknown id returns ErrNotFound. Callers that must not touch deleted entries
// check DeletedAt.
func (s *Store) EntryByID(id uint) (*Entry, error) {
	var e Entry
	if err := s.scoped().First(&e, id).Error; err != nil {
		return nil, translate(err)
	}
	return &e, nil
}

// UpdateEntry writes the columns named in changes for the entry with id, plus
// updated_at from the injected clock. It writes nothing else, so a stop that
// landed after the caller read the entry is not undone by a note edit.
func (s *Store) UpdateEntry(id uint, at time.Time, changes map[string]any) error {
	changes["updated_at"] = at
	return s.scoped().Model(&Entry{}).Where("id = ?", id).UpdateColumns(changes).Error
}

// SoftDeleteEntry sets deleted_at on a live entry; an unknown or already deleted
// id returns ErrNotFound.
func (s *Store) SoftDeleteEntry(id uint, at time.Time) error {
	res := s.scoped().Model(&Entry{}).Where("id = ? AND deleted_at IS NULL", id).
		UpdateColumns(map[string]any{"deleted_at": at, "updated_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreEntry clears deleted_at; an unknown id returns ErrNotFound. A restored
// running entry can collide with one that started since, so that returns
// ErrEntryRunning instead of a driver error.
func (s *Store) RestoreEntry(id uint, at time.Time) error {
	res := s.scoped().Model(&Entry{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"deleted_at": nil, "updated_at": at})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return ErrEntryRunning
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RunningEntry returns the user's running entry in this workspace, or
// ErrNotFound when none runs.
func (s *Store) RunningEntry(userID uint) (*Entry, error) {
	var e Entry
	if err := s.scoped().Where("user_id = ? AND end_at IS NULL AND deleted_at IS NULL", userID).
		First(&e).Error; err != nil {
		return nil, translate(err)
	}
	return &e, nil
}

// StopRunning ends the user's running entry at at, replacing its note when note
// is non-empty. No running entry returns ErrNotFound.
func (s *Store) StopRunning(userID uint, at time.Time, note string) error {
	res := s.scoped().Model(&Entry{}).
		Where("user_id = ? AND end_at IS NULL AND deleted_at IS NULL", userID).
		UpdateColumns(map[string]any{
			"end_at":     at,
			"note":       gorm.Expr("CASE WHEN ? = '' THEN note ELSE ? END", note, note),
			"updated_at": at,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// EntriesInRange returns live entries of this workspace overlapping [from, to),
// oldest first. A scope with IncludeDeleted also returns soft-deleted entries. A
// running entry overlaps every range that starts after its start.
func (s *Store) EntriesInRange(from, to time.Time, scope Scope) ([]Entry, error) {
	out := []Entry{}
	q := s.scoped().Where("start_at < ? AND (end_at IS NULL OR end_at > ?)", to, from)
	// The workspace predicate must survive any OR the scope adds below.
	if !scope.IncludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
	q = scope.apply(q)
	err := q.Order("start_at").Find(&out).Error
	return out, err
}

// apply turns a scope into a WHERE clause; the zero scope matches nothing.
func (sc Scope) apply(q *gorm.DB) *gorm.DB {
	if sc.All {
		return q
	}
	if sc.UserID != 0 && len(sc.ProjectIDs) > 0 {
		return q.Where("(user_id = ? OR project_id IN ?)", sc.UserID, sc.ProjectIDs)
	}
	if sc.UserID != 0 {
		return q.Where("user_id = ?", sc.UserID)
	}
	if len(sc.ProjectIDs) > 0 {
		return q.Where("project_id IN ?", sc.ProjectIDs)
	}
	return q.Where("1 = 0")
}

// ProjectIDsLedBy returns this workspace's projects of every team userID leads;
// archived projects stay in, because their hours still count in reports.
func (s *Store) ProjectIDsLedBy(userID uint) ([]uint, error) {
	out := []uint{}
	err := s.db.Model(&Project{}).
		Where("projects.workspace_id = ?", s.workspaceID).
		Joins("JOIN team_memberships ON team_memberships.team_id = projects.team_id AND team_memberships.workspace_id = projects.workspace_id").
		Joins("JOIN teams ON teams.id = projects.team_id AND teams.workspace_id = projects.workspace_id").
		Where("team_memberships.user_id = ? AND team_memberships.role = ? AND teams.archived_at IS NULL",
			userID, RoleLead).
		Pluck("projects.id", &out).Error
	return out, err
}

// UsersByIDs returns the users with these ids in this workspace; unknown ids are
// simply absent.
func (s *Store) UsersByIDs(ids []uint) ([]User, error) {
	out := []User{}
	if len(ids) == 0 {
		return out, nil
	}
	err := s.scoped().Where("id IN ?", ids).Find(&out).Error
	return out, err
}

// ProjectsByIDs returns this workspace's projects with these ids, archived ones
// included.
func (s *Store) ProjectsByIDs(ids []uint) ([]Project, error) {
	out := []Project{}
	if len(ids) == 0 {
		return out, nil
	}
	err := s.scoped().Where("id IN ?", ids).Find(&out).Error
	return out, err
}

// TeamsByIDs returns this workspace's teams with these ids, archived ones included.
func (s *Store) TeamsByIDs(ids []uint) ([]Team, error) {
	out := []Team{}
	if len(ids) == 0 {
		return out, nil
	}
	err := s.scoped().Where("id IN ?", ids).Find(&out).Error
	return out, err
}
