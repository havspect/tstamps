package gorm

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/plan"
)

// PlanRepository implements plan.Repository over GORM.
type PlanRepository struct {
	db *gorm.DB
}

// NewPlanRepository creates a PlanRepository.
func NewPlanRepository(db *gorm.DB) *PlanRepository {
	return &PlanRepository{db: db}
}

var _ plan.Repository = (*PlanRepository)(nil)

func toPlanDomain(m *Plan) *plan.Plan {
	if m == nil {
		return nil
	}
	return &plan.Plan{
		ID:          m.ID,
		WorkspaceID: m.WorkspaceID,
		UserID:      m.UserID,
		ProjectID:   m.ProjectID,
		WeekStart:   m.WeekStart,
		Hours:       m.Hours,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// People lists every user in the workspace with their teams and capacity.
func (r *PlanRepository) People(ctx context.Context) ([]plan.Person, error) {
	db := DBFrom(ctx, r.db)
	var users []User
	if err := db.Order("name ASC, id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	var memberships []TeamMembership
	if err := db.Find(&memberships).Error; err != nil {
		return nil, err
	}
	var teams []Team
	if err := db.Find(&teams).Error; err != nil {
		return nil, err
	}
	teamNames := make(map[uint]string, len(teams))
	for _, t := range teams {
		teamNames[t.ID] = t.Name
	}
	byUser := make(map[uint][]plan.TeamRef)
	for _, m := range memberships {
		byUser[m.UserID] = append(byUser[m.UserID], plan.TeamRef{ID: m.TeamID, Name: teamNames[m.TeamID]})
	}

	out := make([]plan.Person, len(users))
	for i, u := range users {
		out[i] = plan.Person{
			UserID:      u.ID,
			Name:        u.Name,
			Email:       u.Email,
			Timezone:    u.Timezone,
			WeeklyHours: u.WeeklyHours,
			Active:      u.Active,
			Teams:       byUser[u.ID],
		}
	}
	return out, nil
}

// GetPerson returns one user with their teams and capacity.
func (r *PlanRepository) GetPerson(ctx context.Context, userID uint) (*plan.Person, error) {
	db := DBFrom(ctx, r.db)
	var u User
	if err := db.First(&u, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plan.ErrNotFound
		}
		return nil, err
	}
	var memberships []TeamMembership
	if err := db.Where("user_id = ?", userID).Find(&memberships).Error; err != nil {
		return nil, err
	}
	person := &plan.Person{
		UserID:      u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Timezone:    u.Timezone,
		WeeklyHours: u.WeeklyHours,
		Active:      u.Active,
	}
	for _, m := range memberships {
		var t Team
		if err := db.First(&t, m.TeamID).Error; err == nil {
			person.Teams = append(person.Teams, plan.TeamRef{ID: t.ID, Name: t.Name})
		}
	}
	return person, nil
}

// GetProject returns the project a cell is planned against.
func (r *PlanRepository) GetProject(ctx context.Context, id uint) (*plan.Project, error) {
	db := DBFrom(ctx, r.db)
	var p Project
	if err := db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plan.ErrNotFound
		}
		return nil, err
	}
	return &plan.Project{ID: p.ID, TeamID: p.TeamID, Name: p.Name, ArchivedAt: p.ArchivedAt}, nil
}

// List returns cells with the names and team they need to render.
func (r *PlanRepository) List(ctx context.Context, filter plan.Filter) ([]plan.Cell, error) {
	db := DBFrom(ctx, r.db)
	q := db.Model(&Plan{}).
		Where("week_start >= ? AND week_start < ?", filter.From, filter.To)
	if len(filter.UserIDs) > 0 {
		q = q.Where("user_id IN ?", filter.UserIDs)
	}
	if filter.UserID != 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.ProjectID != 0 {
		q = q.Where("project_id = ?", filter.ProjectID)
	}

	var rows []Plan
	if err := q.Order("week_start ASC, user_id ASC, project_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	userIDs := make([]uint, 0, len(rows))
	projectIDs := make([]uint, 0, len(rows))
	seenUser := map[uint]bool{}
	seenProject := map[uint]bool{}
	for _, row := range rows {
		if !seenUser[row.UserID] {
			seenUser[row.UserID] = true
			userIDs = append(userIDs, row.UserID)
		}
		if !seenProject[row.ProjectID] {
			seenProject[row.ProjectID] = true
			projectIDs = append(projectIDs, row.ProjectID)
		}
	}

	var users []User
	if err := db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	var projects []Project
	if err := db.Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
		return nil, err
	}
	teamIDs := make([]uint, 0, len(projects))
	for _, p := range projects {
		teamIDs = append(teamIDs, p.TeamID)
	}
	var teams []Team
	if len(teamIDs) > 0 {
		if err := db.Where("id IN ?", teamIDs).Find(&teams).Error; err != nil {
			return nil, err
		}
	}

	type userInfo struct{ name, email string }
	usersByID := make(map[uint]userInfo, len(users))
	for _, u := range users {
		usersByID[u.ID] = userInfo{u.Name, u.Email}
	}
	type projectInfo struct {
		name     string
		teamID   uint
		archived bool
	}
	projectsByID := make(map[uint]projectInfo, len(projects))
	for _, p := range projects {
		projectsByID[p.ID] = projectInfo{p.Name, p.TeamID, p.ArchivedAt != nil}
	}
	teamNames := make(map[uint]string, len(teams))
	for _, t := range teams {
		teamNames[t.ID] = t.Name
	}

	out := make([]plan.Cell, 0, len(rows))
	for _, row := range rows {
		u := usersByID[row.UserID]
		p := projectsByID[row.ProjectID]
		if filter.TeamID != 0 && p.teamID != filter.TeamID {
			continue
		}
		out = append(out, plan.Cell{
			UserID:    row.UserID,
			User:      u.name,
			Email:     u.email,
			ProjectID: row.ProjectID,
			Project:   p.name,
			TeamID:    p.teamID,
			Team:      teamNames[p.teamID],
			Archived:  p.archived,
			WeekStart: row.WeekStart,
			Hours:     row.Hours,
		})
	}
	return out, nil
}

// Get returns one cell, or nil when none is stored.
func (r *PlanRepository) Get(ctx context.Context, userID, projectID uint, weekStart time.Time) (*plan.Plan, error) {
	db := DBFrom(ctx, r.db)
	var m Plan
	err := db.Where("user_id = ? AND project_id = ? AND week_start = ?", userID, projectID, weekStart).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toPlanDomain(&m), nil
}

// Upsert writes a cell, keeping the existing row when there is one.
func (r *PlanRepository) Upsert(ctx context.Context, p *plan.Plan) error {
	db := DBFrom(ctx, r.db)
	res := db.Model(&Plan{}).
		Where("user_id = ? AND project_id = ? AND week_start = ?", p.UserID, p.ProjectID, p.WeekStart).
		Updates(map[string]any{"hours": p.Hours, "updated_at": p.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	row := &Plan{
		WorkspaceID: p.WorkspaceID,
		UserID:      p.UserID,
		ProjectID:   p.ProjectID,
		WeekStart:   p.WeekStart,
		Hours:       p.Hours,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if err := db.Create(row).Error; err != nil {
		return err
	}
	p.ID = row.ID
	return nil
}

// Delete removes one cell.
func (r *PlanRepository) Delete(ctx context.Context, userID, projectID uint, weekStart time.Time) error {
	db := DBFrom(ctx, r.db)
	return db.Where("user_id = ? AND project_id = ? AND week_start = ?", userID, projectID, weekStart).
		Delete(&Plan{}).Error
}

// ListVacations returns time off overlapping a date range, newest first.
func (r *PlanRepository) ListVacations(ctx context.Context, filter plan.VacationFilter) ([]plan.Vacation, error) {
	db := DBFrom(ctx, r.db)
	q := db.Model(&Vacation{})
	if filter.UserID != 0 {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if !filter.From.IsZero() {
		q = q.Where("end_date >= ?", plan.Date(filter.From))
	}
	if !filter.To.IsZero() {
		q = q.Where("start_date < ?", plan.Date(filter.To))
	}
	var rows []Vacation
	if err := q.Order("start_date DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]plan.Vacation, len(rows))
	for i, row := range rows {
		out[i] = plan.Vacation{
			ID:          row.ID,
			WorkspaceID: row.WorkspaceID,
			UserID:      row.UserID,
			StartDate:   row.StartDate,
			EndDate:     row.EndDate,
			StartHours:  row.StartHours,
			EndHours:    row.EndHours,
			Note:        row.Note,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
	}
	return out, nil
}

// CreateVacation stores one time-off range.
func (r *PlanRepository) CreateVacation(ctx context.Context, v *plan.Vacation) error {
	db := DBFrom(ctx, r.db)
	row := &Vacation{
		WorkspaceID: v.WorkspaceID,
		UserID:      v.UserID,
		StartDate:   v.StartDate,
		EndDate:     v.EndDate,
		StartHours:  v.StartHours,
		EndHours:    v.EndHours,
		Note:        v.Note,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
	if err := db.Create(row).Error; err != nil {
		return err
	}
	v.ID = row.ID
	return nil
}

// DeleteVacation removes one of a user's ranges. Another user's row is reported
// as missing so an id cannot be probed.
func (r *PlanRepository) DeleteVacation(ctx context.Context, id, userID uint) error {
	db := DBFrom(ctx, r.db)
	res := db.Where("id = ? AND user_id = ?", id, userID).Delete(&Vacation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return plan.ErrNotFound
	}
	return nil
}
