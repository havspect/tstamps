package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/report"
)

var _ report.ReportEntrySource = (*ReportSource)(nil)

// ReportSource implements report.ReportEntrySource by querying GORM models.
type ReportSource struct {
	db *gorm.DB
}

// NewReportSource returns a new ReportSource backed by db.
func NewReportSource(db *gorm.DB) *ReportSource {
	return &ReportSource{db: db}
}

// LoadReportData loads entries, users, projects, and teams required to generate a report.
func (s *ReportSource) LoadReportData(ctx context.Context, filter report.Filter) (report.Input, error) {
	db := DBFrom(ctx, s.db)

	paddedFrom := filter.From.Add(-14 * time.Hour)
	paddedTo := filter.To.Add(14 * time.Hour)

	q := db.Model(&Entry{}).Where("deleted_at IS NULL")
	if !filter.From.IsZero() {
		q = q.Where("end_at IS NULL OR end_at >= ?", paddedFrom)
	}
	if !filter.To.IsZero() {
		q = q.Where("start_at < ?", paddedTo)
	}
	if filter.UserID > 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.ProjectID > 0 {
		q = q.Where("project_id = ?", filter.ProjectID)
	}
	if filter.TeamID > 0 {
		var pIDs []uint
		if err := db.Model(&Project{}).Where("team_id = ?", filter.TeamID).Pluck("id", &pIDs).Error; err != nil {
			return report.Input{}, err
		}
		if len(pIDs) == 0 {
			return report.Input{Now: time.Now()}, nil
		}
		q = q.Where("project_id IN ?", pIDs)
	}
	if len(filter.TeamIDs) > 0 || filter.SelfUserID > 0 {
		var pIDs []uint
		if len(filter.TeamIDs) > 0 {
			if err := db.Model(&Project{}).Where("team_id IN ?", filter.TeamIDs).Pluck("id", &pIDs).Error; err != nil {
				return report.Input{}, err
			}
		}
		cond := db.Where("1 = 0")
		if len(pIDs) > 0 {
			cond = db.Where("project_id IN ?", pIDs)
		}
		if filter.SelfUserID > 0 {
			cond = cond.Or("user_id = ?", filter.SelfUserID)
		}
		q = q.Where(cond)
	}

	var entryRows []Entry
	if err := q.Find(&entryRows).Error; err != nil {
		return report.Input{}, err
	}

	userMap := make(map[uint]struct{})
	projMap := make(map[uint]struct{})
	for _, e := range entryRows {
		userMap[e.UserID] = struct{}{}
		projMap[e.ProjectID] = struct{}{}
	}

	uIDs := make([]uint, 0, len(userMap))
	for uid := range userMap {
		uIDs = append(uIDs, uid)
	}
	pIDs := make([]uint, 0, len(projMap))
	for pid := range projMap {
		pIDs = append(pIDs, pid)
	}

	var userRows []User
	if len(uIDs) > 0 {
		if err := db.Where("id IN ?", uIDs).Find(&userRows).Error; err != nil {
			return report.Input{}, err
		}
	}

	var projRows []Project
	if len(pIDs) > 0 {
		if err := db.Where("id IN ?", pIDs).Find(&projRows).Error; err != nil {
			return report.Input{}, err
		}
	}

	teamMap := make(map[uint]struct{})
	for _, p := range projRows {
		teamMap[p.TeamID] = struct{}{}
	}
	tIDs := make([]uint, 0, len(teamMap))
	for tid := range teamMap {
		tIDs = append(tIDs, tid)
	}

	var teamRows []Team
	if len(tIDs) > 0 {
		if err := db.Where("id IN ?", tIDs).Find(&teamRows).Error; err != nil {
			return report.Input{}, err
		}
	}

	repEntries := make([]report.Entry, len(entryRows))
	for i, e := range entryRows {
		repEntries[i] = report.Entry{
			ID:        e.ID,
			UserID:    e.UserID,
			ProjectID: e.ProjectID,
			StartAt:   e.StartAt,
			EndAt:     e.EndAt,
			Timezone:  e.Timezone,
			Note:      e.Note,
			DeletedAt: e.DeletedAt,
		}
	}

	repUsers := make([]report.User, len(userRows))
	for i, u := range userRows {
		repUsers[i] = report.User{
			ID:    u.ID,
			Name:  u.Name,
			Email: u.Email,
		}
	}

	repProjects := make([]report.Project, len(projRows))
	for i, p := range projRows {
		repProjects[i] = report.Project{
			ID:         p.ID,
			TeamID:     p.TeamID,
			Name:       p.Name,
			ArchivedAt: p.ArchivedAt,
		}
	}

	repTeams := make([]report.Team, len(teamRows))
	for i, t := range teamRows {
		repTeams[i] = report.Team{
			ID:   t.ID,
			Name: t.Name,
		}
	}

	return report.Input{
		Entries:  repEntries,
		Users:    repUsers,
		Projects: repProjects,
		Teams:    repTeams,
		Now:      time.Now(),
	}, nil
}
