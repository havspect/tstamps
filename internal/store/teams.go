package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrUnknownUser is returned when a membership names a user that does not exist.
var ErrUnknownUser = errors.New("unknown user")

// Member is one membership with the user it grants a role to.
type Member struct {
	TeamID uint
	UserID uint
	Email  string
	Name   string
	Role   string
}

// MemberRole is one membership in a replace request.
type MemberRole struct {
	UserID uint
	Role   string
}

// CreateTeam inserts a team into this workspace.
func (s *Store) CreateTeam(t *Team) error {
	return s.db.Create(t).Error
}

// ListTeams returns every team in this workspace, archived ones included,
// ordered by name.
func (s *Store) ListTeams() ([]Team, error) {
	out := []Team{}
	err := s.scoped().Order("name").Find(&out).Error
	return out, err
}

// TeamByID returns the team with id in this workspace, or ErrNotFound.
func (s *Store) TeamByID(id uint) (*Team, error) {
	var t Team
	if err := s.scoped().First(&t, id).Error; err != nil {
		return nil, translate(err)
	}
	return &t, nil
}

// RenameTeam sets the name of team id and returns the updated row; an unknown id
// returns ErrNotFound.
func (s *Store) RenameTeam(id uint, name string, at time.Time) (*Team, error) {
	// UpdateColumns skips the automatic updated_at, so the injected clock decides it.
	res := s.scoped().Model(&Team{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"name": name, "updated_at": at})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.TeamByID(id)
}

// ArchiveTeam archives team id and every project it owns in one transaction; the
// membership rows stay. Archiving an archived team changes nothing, so a repeat
// call succeeds; an unknown id returns ErrNotFound.
func (s *Store) ArchiveTeam(id uint, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&Team{}, id).Error; err != nil {
			return translate(err)
		}
		// The IS NULL guard keeps the first archive time on a repeat call.
		if err := tx.Model(&Team{}).Where("id = ? AND archived_at IS NULL", id).
			UpdateColumns(map[string]any{"archived_at": at, "updated_at": at}).Error; err != nil {
			return err
		}
		return tx.Model(&Project{}).Where("team_id = ? AND archived_at IS NULL", id).
			UpdateColumns(map[string]any{"archived_at": at, "updated_at": at}).Error
	})
}

const memberColumns = "team_memberships.team_id, team_memberships.user_id, users.email, users.name, team_memberships.role"

// AllMembers returns every membership in this workspace with its user, ordered
// by email.
func (s *Store) AllMembers() ([]Member, error) {
	out := []Member{}
	err := s.db.Model(&TeamMembership{}).Select(memberColumns).
		Joins("JOIN users ON users.id = team_memberships.user_id AND users.workspace_id = team_memberships.workspace_id").
		Order("users.email").Scan(&out).Error
	return out, err
}

// MembersOf returns the memberships of one team in this workspace, ordered by email.
func (s *Store) MembersOf(teamID uint) ([]Member, error) {
	out := []Member{}
	err := s.db.Model(&TeamMembership{}).Select(memberColumns).
		Where("team_memberships.team_id = ?", teamID).
		Joins("JOIN users ON users.id = team_memberships.user_id AND users.workspace_id = team_memberships.workspace_id").
		Order("users.email").Scan(&out).Error
	return out, err
}

// ReplaceMemberships makes the membership rows of teamID exactly members in one
// transaction: unlisted rows are deleted, listed ones inserted or given the role.
// An unknown team returns ErrNotFound, an unknown user ErrUnknownUser.
func (s *Store) ReplaceMemberships(teamID uint, members []MemberRole, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&Team{}, teamID).Error; err != nil {
			return translate(err)
		}
		keep := make([]uint, 0, len(members))
		for _, m := range members {
			var n int64
			if err := tx.Model(&User{}).Where("id = ?", m.UserID).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("%w: %d", ErrUnknownUser, m.UserID)
			}
			keep = append(keep, m.UserID)
		}

		// An empty NOT IN list matches nothing, so it needs its own branch.
		del := tx.Where("team_id = ?", teamID)
		if len(keep) > 0 {
			del = del.Where("user_id NOT IN ?", keep)
		}
		if err := del.Delete(&TeamMembership{}).Error; err != nil {
			return err
		}

		for _, m := range members {
			row := TeamMembership{TeamID: teamID, UserID: m.UserID, Role: m.Role, CreatedAt: at}
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"role"}),
			}).Create(&row).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateMembership links a user to a team with role lead or member.
func (s *Store) CreateMembership(m *TeamMembership) error {
	return s.db.Create(m).Error
}
