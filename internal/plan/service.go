package plan

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/report"
)

// Service enforces the planning rules over the repository.
type Service struct {
	repo       Repository
	uow        UnitOfWork
	audit      AuditRecorder
	reports    ReportSource
	authorizer authz.Authorizer
	clock      clock.Clock
}

// NewService builds a plan Service.
func NewService(
	repo Repository,
	uow UnitOfWork,
	audit AuditRecorder,
	reports ReportSource,
	authorizer authz.Authorizer,
	clk clock.Clock,
) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{repo: repo, uow: uow, audit: audit, reports: reports, authorizer: authorizer, clock: clk}
}

// SeedInput fills a month's plan from the previous month's logged hours.
type SeedInput struct {
	FromMonth string
	IntoMonth string
	TeamID    uint
	DryRun    bool
}

// RepeatInput copies one week's plan into the weeks after it.
type RepeatInput struct {
	WeekStart time.Time
	Weeks     int
	TeamID    uint
	UserID    uint
	ProjectID uint
	DryRun    bool
}

func principal(ctx context.Context) (*auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return nil, authz.ErrUnauthorized
	}
	return p, nil
}

func hasTeam(person Person, teamID uint) bool {
	for _, t := range person.Teams {
		if t.ID == teamID {
			return true
		}
	}
	return false
}

// mayPlanPerson is the half of the rule authz cannot see: a lead plans people
// of a team they lead, plus themselves.
func mayPlanPerson(p *auth.Principal, person *Person) bool {
	if p.IsAdmin() || person.UserID == p.UserID {
		return true
	}
	for _, teamID := range p.LedTeamIDs {
		if hasTeam(*person, teamID) {
			return true
		}
	}
	return false
}

// mayReadVacations lets a person read their own time off, an admin read any,
// and a lead read anyone in a team they lead.
func mayReadVacations(p *auth.Principal, person *Person) bool {
	if p.IsAdmin() || person.UserID == p.UserID {
		return true
	}
	for _, teamID := range p.LedTeamIDs {
		if hasTeam(*person, teamID) {
			return true
		}
	}
	return false
}

// Readable reports whether the caller may see this cell with its project name.
// Cells they may not see are summed into the board's "other" hours instead.
func (s *Service) readable(p *auth.Principal, cell Cell) bool {
	return p.IsAdmin() || cell.UserID == p.UserID || p.LeadsTeam(cell.TeamID)
}

// List returns the board for a range, narrowed to what the caller may read.
func (s *Service) List(ctx context.Context, filter Filter) (*Board, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) {
		return nil, ErrInvalidRange
	}

	people, err := s.repo.People(ctx)
	if err != nil {
		return nil, err
	}
	if filter.TeamID != 0 && !p.IsAdmin() && !p.LeadsTeam(filter.TeamID) {
		return nil, authz.ErrForbidden
	}
	if filter.UserID != 0 && filter.UserID != p.UserID {
		if !p.IsAdmin() && !p.HasRole("lead") {
			return nil, authz.ErrForbidden
		}
	}

	eligible := make(map[uint]Person, len(people))
	for _, person := range people {
		switch {
		case filter.TeamID != 0 && !hasTeam(person, filter.TeamID):
		case filter.UserID != 0 && person.UserID != filter.UserID:
		case p.IsAdmin() || person.UserID == p.UserID:
			eligible[person.UserID] = person
		case p.HasRole("lead"):
			if mayPlanPerson(p, &person) {
				eligible[person.UserID] = person
			}
		}
	}

	all, err := s.repo.List(ctx, Filter{
		From:      filter.From,
		To:        filter.To,
		TeamID:    filter.TeamID,
		ProjectID: filter.ProjectID,
	})
	if err != nil {
		return nil, err
	}

	board := &Board{Weeks: WeeksIn(filter.From, filter.To)}
	vacations, err := s.repo.ListVacations(ctx, VacationFilter{From: filter.From, To: filter.To})
	if err != nil {
		return nil, err
	}
	byUser := map[uint][]Vacation{}
	for _, v := range vacations {
		byUser[v.UserID] = append(byUser[v.UserID], v)
	}
	shown := make(map[uint]bool, len(eligible))
	other := map[uint]map[time.Time]int{}
	for _, cell := range all {
		if _, ok := eligible[cell.UserID]; !ok {
			continue
		}
		if !s.readable(p, cell) {
			if other[cell.UserID] == nil {
				other[cell.UserID] = map[time.Time]int{}
			}
			other[cell.UserID][cell.WeekStart] += cell.Hours
			shown[cell.UserID] = true
			continue
		}
		board.Cells = append(board.Cells, cell)
		shown[cell.UserID] = true
	}

	for _, person := range eligible {
		if !person.Active && !shown[person.UserID] {
			// A leaver keeps a row only while hours they were planned for remain.
			continue
		}
		if !person.Active {
			person.WeeklyHours = 0
		}
		board.People = append(board.People, person)
	}
	sort.Slice(board.People, func(i, j int) bool { return board.People[i].Name < board.People[j].Name })
	for _, person := range board.People {
		for _, week := range board.Weeks {
			hours := VacationHours(week, person.WeeklyHours, byUser[person.UserID])
			board.PersonWeeks = append(board.PersonWeeks, PersonWeek{
				UserID:        person.UserID,
				WeekStart:     week,
				Capacity:      WeeklyCapacity(person.WeeklyHours, hours),
				VacationHours: hours,
			})
		}
	}

	for userID, weeks := range other {
		for week, hours := range weeks {
			board.Other = append(board.Other, OtherHours{UserID: userID, WeekStart: week, Hours: hours})
		}
	}
	sort.Slice(board.Other, func(i, j int) bool {
		if board.Other[i].UserID != board.Other[j].UserID {
			return board.Other[i].UserID < board.Other[j].UserID
		}
		return board.Other[i].WeekStart.Before(board.Other[j].WeekStart)
	})
	return board, nil
}

// Set writes one cell. Zero hours deletes it, and deleting a missing cell is a
// no-op so an agent can retry safely.
func (s *Service) Set(ctx context.Context, userID, projectID uint, weekStart time.Time, hours int) (*Plan, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if !ValidHours(hours) {
		return nil, ErrInvalidHours
	}
	week := WeekStart(weekStart)

	person, err := s.repo.GetPerson(ctx, userID)
	if err != nil {
		return nil, err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project.ArchivedAt != nil {
		return nil, ErrProjectNotPlannable
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionManagePlan, authz.PlanResource{UserID: userID, TeamID: project.TeamID}); err != nil {
		return nil, err
	}
	if !mayPlanPerson(p, person) {
		return nil, authz.ErrForbidden
	}
	now := s.clock.Now()
	if Ended(week, person.Timezone, now) {
		return nil, ErrFrozenWeek
	}

	cell := &Plan{
		WorkspaceID: p.WorkspaceID,
		UserID:      userID,
		ProjectID:   projectID,
		WeekStart:   week,
		Hours:       hours,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		before, err := s.repo.Get(txCtx, userID, projectID, week)
		if err != nil {
			return err
		}
		if before != nil {
			cell.ID = before.ID
			cell.CreatedAt = before.CreatedAt
		}
		switch {
		case before == nil && hours == 0:
			return nil
		case hours == 0:
			if err := s.repo.Delete(txCtx, userID, projectID, week); err != nil {
				return err
			}
			return s.record(txCtx, "plan.delete", before.ID, map[string]any{"before": before.AuditFields()})
		default:
			if err := s.repo.Upsert(txCtx, cell); err != nil {
				return err
			}
			if before == nil {
				return s.record(txCtx, "plan.create", cell.ID, map[string]any{"after": cell.AuditFields()})
			}
			return s.record(txCtx, "plan.update", cell.ID, map[string]any{
				"before": before.AuditFields(),
				"after":  cell.AuditFields(),
			})
		}
	})
	if err != nil {
		return nil, err
	}
	return cell, nil
}

// VacationInput is a date range a user marks as time off.
type VacationInput struct {
	StartDate  time.Time
	EndDate    time.Time
	StartHours float64
	EndHours   float64
	Note       string
}

// VacationQuery narrows a vacation list to one person and optional dates. A
// zero user means the caller.
type VacationQuery struct {
	UserID uint
	From   time.Time
	To     time.Time
}

// ListVacations returns readable vacations, newest first: the caller's own, a
// lead's team members', or every person's for an admin.
func (s *Service) ListVacations(ctx context.Context, q VacationQuery) ([]Vacation, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	target := q.UserID
	if target == 0 {
		target = p.UserID
	}
	person, err := s.repo.GetPerson(ctx, target)
	if err != nil {
		return nil, err
	}
	if !mayReadVacations(p, person) {
		return nil, authz.ErrForbidden
	}
	vacations, err := s.repo.ListVacations(ctx, VacationFilter{UserID: target, From: q.From, To: q.To})
	if err != nil {
		return nil, err
	}
	for i := range vacations {
		vacations[i].User = person.Name
		vacations[i].Email = person.Email
	}
	return vacations, nil
}

// validVacationHours reports whether h is a positive, one-decimal number inside
// one working day.
func validVacationHours(h, daily float64) bool {
	if h <= 0 || h > daily+1e-9 {
		return false
	}
	return math.Abs(h*10-math.Round(h*10)) < 1e-6
}

// AddVacation records time off for the caller. Any signed-in user may add their
// own; nobody records leave for someone else. A first or last date on a weekend
// keeps zero hours because weekends never lower capacity.
func (s *Service) AddVacation(ctx context.Context, in VacationInput) (*Vacation, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionManageVacation, authz.VacationResource{UserID: p.UserID}); err != nil {
		return nil, err
	}
	start, end := Date(in.StartDate), Date(in.EndDate)
	if start.IsZero() || end.IsZero() || end.Before(start) || end.After(start.AddDate(0, 0, MaxVacationDays-1)) {
		return nil, ErrInvalidVacation
	}
	person, err := s.repo.GetPerson(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	startHours, endHours := in.StartHours, in.EndHours
	daily := DailyHours(person.WeeklyHours)
	if !IsWorkingDay(start) {
		startHours = 0
	} else if !validVacationHours(startHours, daily) {
		return nil, ErrInvalidVacation
	}
	if start.Equal(end) {
		endHours = 0
	} else if !IsWorkingDay(end) {
		endHours = 0
	} else if !validVacationHours(endHours, daily) {
		return nil, ErrInvalidVacation
	}

	now := s.clock.Now()
	v := &Vacation{
		WorkspaceID: p.WorkspaceID,
		UserID:      p.UserID,
		User:        person.Name,
		Email:       person.Email,
		StartDate:   start,
		EndDate:     end,
		StartHours:  startHours,
		EndHours:    endHours,
		Note:        strings.TrimSpace(in.Note),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		existing, err := s.repo.ListVacations(txCtx, VacationFilter{
			UserID: p.UserID, From: start, To: end.AddDate(0, 0, 1),
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return ErrVacationOverlap
		}
		return s.repo.CreateVacation(txCtx, v)
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

// DeleteVacation removes one of the caller's own vacations.
func (s *Service) DeleteVacation(ctx context.Context, id uint) error {
	p, err := principal(ctx)
	if err != nil {
		return err
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionManageVacation, authz.VacationResource{UserID: p.UserID}); err != nil {
		return err
	}
	return s.repo.DeleteVacation(ctx, id, p.UserID)
}

// Seed splits the previous month's logged hours per person and project evenly
// across the target month's weeks, filling empty cells only.
func (s *Service) Seed(ctx context.Context, in SeedInput) (*Summary, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	fromStart, fromEnd, err := MonthStart(in.FromMonth)
	if err != nil {
		return nil, err
	}
	intoStart, intoEnd, err := MonthStart(in.IntoMonth)
	if err != nil {
		return nil, err
	}
	weeks := WeeksIn(intoStart, intoEnd)
	if len(weeks) == 0 {
		return nil, ErrUnknownMonth
	}

	rep, err := s.reports.GenerateReport(ctx, report.Filter{
		From: fromStart, To: fromEnd, By: "none", TeamID: in.TeamID,
	})
	if err != nil {
		return nil, err
	}

	type key struct{ user, project uint }
	totals := map[key]int64{}
	for _, row := range rep.Rows {
		totals[key{row.UserID, row.ProjectID}] += row.Seconds
	}
	keys := make([]key, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].user != keys[j].user {
			return keys[i].user < keys[j].user
		}
		return keys[i].project < keys[j].project
	})

	sum := &Summary{}
	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		people := map[uint]*Person{}
		for _, k := range keys {
			hours := int(math.Round(float64(totals[k]) / 3600 / float64(len(weeks))))
			if hours < 1 {
				sum.Skipped += len(weeks)
				continue
			}
			person := people[k.user]
			if person == nil {
				person, err = s.repo.GetPerson(txCtx, k.user)
				if err != nil {
					return err
				}
				people[k.user] = person
			}
			project, err := s.repo.GetProject(txCtx, k.project)
			if err != nil {
				return err
			}
			for _, week := range weeks {
				if err := s.fill(txCtx, p, person, project.TeamID, k.user, k.project, week, hours, in.DryRun, sum); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sum, nil
}

// Repeat copies one week's visible plan into the weeks after it.
func (s *Service) Repeat(ctx context.Context, in RepeatInput) (*Summary, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if in.Weeks < 1 || in.Weeks > MaxRepeatWeeks {
		return nil, ErrInvalidHours
	}
	week := WeekStart(in.WeekStart)

	people, err := s.repo.People(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(people))
	for _, person := range people {
		if person.Active && (p.IsAdmin() || mayPlanPerson(p, &person) || person.UserID == p.UserID) {
			ids = append(ids, person.UserID)
		}
	}
	source, err := s.repo.List(ctx, Filter{
		From: week, To: week.AddDate(0, 0, 7),
		UserIDs: ids, TeamID: in.TeamID, UserID: in.UserID, ProjectID: in.ProjectID,
	})
	if err != nil {
		return nil, err
	}

	sum := &Summary{}
	err = s.uow.Run(ctx, func(txCtx context.Context) error {
		cache := map[uint]*Person{}
		for _, cell := range source {
			if cell.Hours == 0 || !s.readable(p, cell) {
				continue
			}
			person := cache[cell.UserID]
			if person == nil {
				person, err = s.repo.GetPerson(txCtx, cell.UserID)
				if err != nil {
					return err
				}
				cache[cell.UserID] = person
			}
			for offset := 1; offset <= in.Weeks; offset++ {
				target := week.AddDate(0, 0, 7*offset)
				if err := s.fill(txCtx, p, person, cell.TeamID, cell.UserID, cell.ProjectID, target, cell.Hours, in.DryRun, sum); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sum, nil
}

// fill writes one cell of a bulk, skipping frozen weeks, existing cells and
// cells the caller may not write.
func (s *Service) fill(
	ctx context.Context,
	p *auth.Principal,
	person *Person,
	teamID, userID, projectID uint,
	week time.Time,
	hours int,
	dryRun bool,
	sum *Summary,
) error {
	if Ended(week, person.Timezone, s.clock.Now()) {
		sum.Skipped++
		return nil
	}
	if !mayPlanPerson(p, person) {
		sum.Skipped++
		return nil
	}
	if err := s.authorizer.Check(ctx, p, authz.ActionManagePlan, authz.PlanResource{UserID: userID, TeamID: teamID}); err != nil {
		if err == authz.ErrForbidden {
			sum.Skipped++
			return nil
		}
		return err
	}
	before, err := s.repo.Get(ctx, userID, projectID, week)
	if err != nil {
		return err
	}
	if before != nil || hours <= 0 {
		sum.Skipped++
		return nil
	}

	now := s.clock.Now()
	cell := &Plan{
		WorkspaceID: p.WorkspaceID, UserID: userID, ProjectID: projectID,
		WeekStart: week, Hours: hours, CreatedAt: now, UpdatedAt: now,
	}
	sum.Filled++
	sum.Cells = append(sum.Cells, Cell{UserID: userID, ProjectID: projectID, WeekStart: week, Hours: hours})
	if dryRun {
		return nil
	}
	if err := s.repo.Upsert(ctx, cell); err != nil {
		return err
	}
	return s.record(ctx, "plan.create", cell.ID, map[string]any{"after": cell.AuditFields()})
}

func (s *Service) record(ctx context.Context, action string, targetID uint, changes any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Record(ctx, action, "plan", targetID, changes)
}
