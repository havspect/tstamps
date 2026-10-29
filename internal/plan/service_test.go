package plan

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/report"
)

type fakeRepo struct {
	cells    map[string]*Plan
	people   map[uint]*Person
	projects map[uint]*Project
	nextID   uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		cells:    map[string]*Plan{},
		people:   map[uint]*Person{},
		projects: map[uint]*Project{},
		nextID:   1,
	}
}

func cellKey(userID, projectID uint, week time.Time) string {
	return fmt.Sprintf("%d-%d-%s", userID, projectID, week.Format("2006-01-02"))
}

func (r *fakeRepo) List(_ context.Context, f Filter) ([]Cell, error) {
	var out []Cell
	for _, p := range r.cells {
		if p.WeekStart.Before(f.From) || !p.WeekStart.Before(f.To) {
			continue
		}
		if f.UserID != 0 && p.UserID != f.UserID {
			continue
		}
		if f.ProjectID != 0 && p.ProjectID != f.ProjectID {
			continue
		}
		if len(f.UserIDs) > 0 && !contains(f.UserIDs, p.UserID) {
			continue
		}
		project := r.projects[p.ProjectID]
		if project == nil || (f.TeamID != 0 && project.TeamID != f.TeamID) {
			continue
		}
		person := r.people[p.UserID]
		out = append(out, Cell{
			UserID: p.UserID, User: person.Name, Email: person.Email,
			ProjectID: p.ProjectID, Project: project.Name,
			TeamID: project.TeamID, Archived: project.ArchivedAt != nil,
			WeekStart: p.WeekStart, Hours: p.Hours,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].WeekStart.Equal(out[j].WeekStart) {
			return out[i].WeekStart.Before(out[j].WeekStart)
		}
		if out[i].UserID != out[j].UserID {
			return out[i].UserID < out[j].UserID
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	return out, nil
}

func contains(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (r *fakeRepo) People(context.Context) ([]Person, error) {
	out := make([]Person, 0, len(r.people))
	for _, p := range r.people {
		out = append(out, *p)
	}
	return out, nil
}

func (r *fakeRepo) GetPerson(_ context.Context, id uint) (*Person, error) {
	p, ok := r.people[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *fakeRepo) GetProject(_ context.Context, id uint) (*Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *fakeRepo) Get(_ context.Context, userID, projectID uint, week time.Time) (*Plan, error) {
	return r.cells[cellKey(userID, projectID, week)], nil
}

func (r *fakeRepo) Upsert(_ context.Context, p *Plan) error {
	if p.ID == 0 {
		p.ID = r.nextID
		r.nextID++
	}
	stored := *p
	r.cells[cellKey(p.UserID, p.ProjectID, p.WeekStart)] = &stored
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, userID, projectID uint, week time.Time) error {
	delete(r.cells, cellKey(userID, projectID, week))
	return nil
}

type fakeUOW struct{}

func (fakeUOW) Run(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeAudit struct{ events int }

func (a *fakeAudit) Record(context.Context, string, string, uint, any) error {
	a.events++
	return nil
}

type fakeReports struct{ rows []report.Row }

func (r fakeReports) GenerateReport(context.Context, report.Filter) (*report.Report, error) {
	return &report.Report{Rows: r.rows}, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func leadCtx(userID uint, teams ...uint) context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1, UserID: userID,
		Roles:      []string{"lead"},
		LedTeamIDs: teams,
	})
}

type fixture struct {
	repo  *fakeRepo
	svc   *Service
	audit *fakeAudit
}

func newFixture(now time.Time) *fixture {
	repo := newFakeRepo()
	repo.people[1] = &Person{UserID: 1, Name: "Ana", Email: "ana@example.com", Timezone: "UTC", WeeklyHours: 40, Active: true,
		Teams: []TeamRef{{ID: 1, Name: "Platform"}}}
	repo.people[2] = &Person{UserID: 2, Name: "Bo", Email: "bo@example.com", Timezone: "UTC", WeeklyHours: 40, Active: true,
		Teams: []TeamRef{{ID: 1, Name: "Platform"}}}
	repo.people[3] = &Person{UserID: 3, Name: "Cid", Email: "cid@example.com", Timezone: "UTC", WeeklyHours: 40, Active: true,
		Teams: []TeamRef{{ID: 2, Name: "Design"}}}
	repo.people[4] = &Person{UserID: 4, Name: "Dee", Email: "dee@example.com", Timezone: "UTC", WeeklyHours: 40, Active: false,
		Teams: []TeamRef{{ID: 1, Name: "Platform"}}}
	repo.people[5] = &Person{UserID: 5, Name: "Eli", Email: "eli@example.com", Timezone: "UTC", WeeklyHours: 40, Active: false,
		Teams: []TeamRef{{ID: 1, Name: "Platform"}}}
	repo.projects[10] = &Project{ID: 10, TeamID: 1, Name: "Timeline"}
	repo.projects[20] = &Project{ID: 20, TeamID: 2, Name: "Billing"}
	audit := &fakeAudit{}
	return &fixture{
		repo:  repo,
		audit: audit,
		svc:   NewService(repo, fakeUOW{}, audit, fakeReports{}, authz.NewPolicyEngine(), fixedClock{now}),
	}
}

func TestWeekStartAndWeeksIn(t *testing.T) {
	// 2026-11-04 is a Wednesday; its week starts Monday 2026-11-02.
	if got := WeekStart(time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)); got.Format("2006-01-02") != "2026-11-02" {
		t.Fatalf("WeekStart = %s", got.Format("2006-01-02"))
	}
	weeks := WeeksIn(
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(weeks) != 5 {
		t.Fatalf("November 2026 has %d Mondays, want 5", len(weeks))
	}
}

func TestSetWritesAndDeletes(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(1, 1)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	cell, err := f.svc.Set(ctx, 2, 10, week, 20)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if cell.Hours != 20 || cell.ID == 0 {
		t.Fatalf("cell = %+v", cell)
	}
	if f.audit.events != 1 {
		t.Fatalf("audit events = %d, want 1", f.audit.events)
	}

	if _, err := f.svc.Set(ctx, 2, 10, week, 0); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if stored, _ := f.repo.Get(ctx, 2, 10, week); stored != nil {
		t.Fatalf("cell survived zero hours: %+v", stored)
	}
	// Deleting a missing cell is a no-op, so a retry is safe.
	if _, err := f.svc.Set(ctx, 2, 10, week, 0); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestSetRejectsBoundsFrozenWeeksAndForeignTeams(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(1, 1)
	next := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	if _, err := f.svc.Set(ctx, 2, 10, next, 169); !errors.Is(err, ErrInvalidHours) {
		t.Fatalf("169 hours: %v", err)
	}
	if _, err := f.svc.Set(ctx, 2, 10, next, -1); !errors.Is(err, ErrInvalidHours) {
		t.Fatalf("-1 hours: %v", err)
	}
	past := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if _, err := f.svc.Set(ctx, 2, 10, past, 4); !errors.Is(err, ErrFrozenWeek) {
		t.Fatalf("past week: %v", err)
	}
	if _, err := f.svc.Set(ctx, 2, 20, next, 4); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("foreign team's project: %v", err)
	}
	// Cid is not in a team the lead leads, and the lead is not planning herself.
	if _, err := f.svc.Set(ctx, 3, 10, next, 4); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("foreign person: %v", err)
	}
}

func TestSeedSplitsEvenlyAndFillsOnlyEmptyCells(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	f.svc.reports = fakeReports{rows: []report.Row{
		{UserID: 2, ProjectID: 10, Seconds: 40 * 3600},
	}}
	ctx := leadCtx(1, 1)

	sum, err := f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	// 40 hours over November's five weeks is 8 hours a week.
	if sum.Filled != 5 || sum.Skipped != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	for _, cell := range sum.Cells {
		if cell.Hours != 8 {
			t.Fatalf("seeded %d hours, want 8", cell.Hours)
		}
	}

	again, err := f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
	if err != nil {
		t.Fatalf("Seed again: %v", err)
	}
	if again.Filled != 0 || again.Skipped != 5 {
		t.Fatalf("second seed = %+v", again)
	}
}

func TestSeedDryRunWritesNothing(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	f.svc.reports = fakeReports{rows: []report.Row{{UserID: 2, ProjectID: 10, Seconds: 20 * 3600}}}

	sum, err := f.svc.Seed(leadCtx(1, 1), SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11", DryRun: true})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if sum.Filled != 5 {
		t.Fatalf("dry run filled = %d, want 5", sum.Filled)
	}
	if len(f.repo.cells) != 0 {
		t.Fatalf("dry run wrote %d cells", len(f.repo.cells))
	}
	if f.audit.events != 0 {
		t.Fatalf("dry run audited %d events", f.audit.events)
	}
}

func TestRepeatNeverOverwrites(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(1, 1)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if _, err := f.svc.Set(ctx, 2, 10, week, 6); err != nil {
		t.Fatalf("set source: %v", err)
	}
	if _, err := f.svc.Set(ctx, 2, 10, week.AddDate(0, 0, 7), 10); err != nil {
		t.Fatalf("set second week: %v", err)
	}

	sum, err := f.svc.Repeat(ctx, RepeatInput{WeekStart: week, Weeks: 2})
	if err != nil {
		t.Fatalf("Repeat: %v", err)
	}
	if sum.Filled != 1 || sum.Skipped != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if stored, _ := f.repo.Get(ctx, 2, 10, week.AddDate(0, 0, 7)); stored.Hours != 10 {
		t.Fatalf("existing cell overwritten with %d", stored.Hours)
	}
	if stored, _ := f.repo.Get(ctx, 2, 10, week.AddDate(0, 0, 14)); stored.Hours != 6 {
		t.Fatalf("third week = %+v", stored)
	}
}

func TestListNarrowsToTheLeadsTeamsAndSumsOtherLoad(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, cell := range []struct {
		user, project uint
		hours         int
	}{{2, 10, 8}, {2, 20, 6}, {3, 20, 4}, {4, 10, 5}} {
		if err := f.repo.Upsert(context.Background(), &Plan{
			WorkspaceID: 1, UserID: cell.user, ProjectID: cell.project, WeekStart: week, Hours: cell.hours,
		}); err != nil {
			t.Fatalf("seed cell: %v", err)
		}
	}

	board, err := f.svc.List(leadCtx(1, 1), Filter{From: week, To: week.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Ana (self), Bo, and Dee, who is deactivated but still has a cell. Eli is
	// deactivated with no cells and does not appear.
	if len(board.People) != 3 || board.People[0].UserID != 1 || board.People[1].UserID != 2 || board.People[2].UserID != 4 {
		t.Fatalf("people = %+v", board.People)
	}
	if board.People[2].WeeklyHours != 0 {
		t.Fatalf("deactivated person keeps capacity: %+v", board.People[2])
	}
	if len(board.Cells) != 2 || board.Cells[1].UserID != 4 {
		t.Fatalf("cells = %+v", board.Cells)
	}
	if len(board.Other) != 1 || board.Other[0].Hours != 6 {
		t.Fatalf("other = %+v", board.Other)
	}
}
