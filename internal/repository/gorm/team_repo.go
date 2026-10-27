package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/havspect/tstamps/internal/team"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TeamRepository provides GORM persistence for teams and memberships.
type TeamRepository struct {
	db *gorm.DB
}

// NewTeamRepository returns a new TeamRepository backed by db.
func NewTeamRepository(db *gorm.DB) *TeamRepository {
	return &TeamRepository{db: db}
}

var (
	_ team.Repository      = (*TeamRepository)(nil)
	_ team.ProjectArchiver = (*TeamRepository)(nil)
)

func toDomainTeam(t *Team) *team.Team {
	if t == nil {
		return nil
	}
	return &team.Team{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		Name:        t.Name,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		ArchivedAt:  t.ArchivedAt,
	}
}

// GetByID returns the team with id, or team.ErrNotFound.
func (r *TeamRepository) GetByID(ctx context.Context, id uint) (*team.Team, error) {
	var row Team
	if err := DBFrom(ctx, r.db).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, team.ErrNotFound
		}
		return nil, err
	}
	return toDomainTeam(&row), nil
}

// List returns all teams in the workspace, ordered by name.
func (r *TeamRepository) List(ctx context.Context, includeArchived bool) ([]team.TeamWithMembers, error) {
	db := DBFrom(ctx, r.db)

	q := db.Model(&Team{})
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}

	var teamRows []Team
	if err := q.Order("name ASC").Find(&teamRows).Error; err != nil {
		return nil, err
	}

	var memRows []TeamMembership
	if err := db.Order("id ASC").Find(&memRows).Error; err != nil {
		return nil, err
	}

	byTeam := make(map[uint][]team.Membership, len(teamRows))
	for _, m := range memRows {
		byTeam[m.TeamID] = append(byTeam[m.TeamID], team.Membership{
			ID:          m.ID,
			WorkspaceID: m.WorkspaceID,
			TeamID:      m.TeamID,
			UserID:      m.UserID,
			Role:        m.Role,
			CreatedAt:   m.CreatedAt,
		})
	}

	out := make([]team.TeamWithMembers, len(teamRows))
	for i, t := range teamRows {
		members := byTeam[t.ID]
		if members == nil {
			members = []team.Membership{}
		}
		out[i] = team.TeamWithMembers{
			Team:    *toDomainTeam(&t),
			Members: members,
		}
	}

	return out, nil
}

// Create inserts a new team into the workspace.
func (r *TeamRepository) Create(ctx context.Context, t *team.Team) error {
	row := Team{
		WorkspaceID: t.WorkspaceID,
		Name:        t.Name,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		ArchivedAt:  t.ArchivedAt,
	}

	if err := DBFrom(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}

	t.ID = row.ID
	t.WorkspaceID = row.WorkspaceID
	return nil
}

// Update updates team name and timestamps.
func (r *TeamRepository) Update(ctx context.Context, t *team.Team) error {
	res := DBFrom(ctx, r.db).Model(&Team{}).Where("id = ?", t.ID).Updates(map[string]any{
		"name":        t.Name,
		"updated_at":  t.UpdatedAt,
		"archived_at": t.ArchivedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return team.ErrNotFound
	}
	return nil
}

// Archive archives team id at the specified time.
func (r *TeamRepository) Archive(ctx context.Context, id uint, at time.Time) error {
	var row Team
	if err := DBFrom(ctx, r.db).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return team.ErrNotFound
		}
		return err
	}

	return DBFrom(ctx, r.db).Model(&Team{}).
		Where("id = ? AND archived_at IS NULL", id).
		Updates(map[string]any{
			"archived_at": at,
			"updated_at":  at,
		}).Error
}

// ReplaceMemberships replaces all memberships of teamID in one transaction.
func (r *TeamRepository) ReplaceMemberships(ctx context.Context, teamID uint, members []team.Membership) error {
	db := DBFrom(ctx, r.db)

	return db.Transaction(func(tx *gorm.DB) error {
		var t Team
		if err := tx.First(&t, teamID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return team.ErrNotFound
			}
			return err
		}

		keep := make([]uint, 0, len(members))
		for _, m := range members {
			var count int64
			if err := tx.Model(&User{}).Where("id = ?", m.UserID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("%w: %d", team.ErrUnknownUser, m.UserID)
			}
			keep = append(keep, m.UserID)
		}

		del := tx.Where("team_id = ?", teamID)
		if len(keep) > 0 {
			del = del.Where("user_id NOT IN ?", keep)
		}
		if err := del.Delete(&TeamMembership{}).Error; err != nil {
			return err
		}

		for _, m := range members {
			row := TeamMembership{
				WorkspaceID: m.WorkspaceID,
				TeamID:      teamID,
				UserID:      m.UserID,
				Role:        m.Role,
				CreatedAt:   m.CreatedAt,
			}
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

// ArchiveProjectsByTeam cascades archiving to all active projects owned by teamID.
func (r *TeamRepository) ArchiveProjectsByTeam(ctx context.Context, teamID uint, at time.Time) error {
	return DBFrom(ctx, r.db).Model(&Project{}).
		Where("team_id = ? AND archived_at IS NULL", teamID).
		Updates(map[string]any{
			"archived_at": at,
			"updated_at":  at,
		}).Error
}
