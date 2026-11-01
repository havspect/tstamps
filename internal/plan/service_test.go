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
	cells     map[string]*Plan
	people    map[uint]*Person
	projects  map[uint]*Project
	vacations []Vacation
	nextID    uint
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

func (r *fakeRepo) ListVacations(_ context.Context, f VacationFilter) ([]Vacation, error) {
	var out []Vacation
	for _, v := range r.vacations {
		if f.UserID != 0 && v.UserID != f.UserID {
			continue
		}
		if !f.From.IsZero() && v.EndDate.Before(Date(f.From)) {
			continue
		}
		if !f.To.IsZero() && !v.StartDate.Before(Date(f.To)) {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeRepo) CreateVacation(_ context.Context, v *Vacation) error {
	v.ID = r.nextID
	r.nextID++
	r.vacations = append(r.vacations, *v)
	return nil
}

func (r *fakeRepo) DeleteVacation(_ context.Context, id, userID uint) error {
	for i, v := range r.vacations {
		if v.ID == id && v.UserID == userID {
			r.vacations = append(r.vacations[:i], r.vacations[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
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

func adminCtx(userID uint) context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1, UserID: userID, Roles: []string{"admin"},
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

// A cell may only name a project of the planned person's teams (ADR-0023).
func TestSetKeepsCellsInsideThePersonsTeams(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	admin := adminCtx(9)

	// Cid is in Design only, so even an admin cannot plan her onto Platform work.
	if _, err := f.svc.Set(admin, 3, 10, week, 4); !errors.Is(err, ErrProjectNotInPersonTeams) {
		t.Fatalf("cross-team project: %v", err)
	}
	if len(f.repo.cells) != 0 {
		t.Fatalf("refused write stored %d cells", len(f.repo.cells))
	}
	if _, err := f.svc.Set(admin, 3, 20, week, 4); err != nil {
		t.Fatalf("own team's project: %v", err)
	}

	// Permission still comes first: a lead naming a foreign project is refused by
	// the authorizer, not by the team rule.
	if _, err := f.svc.Set(leadCtx(1, 1), 2, 20, week, 4); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("lead on a foreign project: %v", err)
	}

	// Zero hours still clears a cross-team cell written before this rule.
	if err := f.repo.Upsert(context.Background(), &Plan{WorkspaceID: 1, UserID: 2, ProjectID: 20, WeekStart: week, Hours: 5}); err != nil {
		t.Fatalf("seed legacy cell: %v", err)
	}
	if _, err := f.svc.Set(admin, 2, 20, week, 0); err != nil {
		t.Fatalf("delete cross-team cell: %v", err)
	}
	if stored, _ := f.repo.Get(context.Background(), 2, 20, week); stored != nil {
		t.Fatalf("cross-team cell survived zero hours: %+v", stored)
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
	if len(sum.Skips) != 1 || sum.Skips[0].Reason != SkipOccupied {
		t.Fatalf("skips = %+v, want one occupied", sum.Skips)
	}
	if !sum.Skips[0].WeekStart.Equal(week.AddDate(0, 0, 7)) {
		t.Fatalf("occupied week = %s", sum.Skips[0].WeekStart.Format("2006-01-02"))
	}
	if stored, _ := f.repo.Get(ctx, 2, 10, week.AddDate(0, 0, 7)); stored.Hours != 10 {
		t.Fatalf("existing cell overwritten with %d", stored.Hours)
	}
	if stored, _ := f.repo.Get(ctx, 2, 10, week.AddDate(0, 0, 14)); stored.Hours != 6 {
		t.Fatalf("third week = %+v", stored)
	}
}

func TestRepeatRejectsWeekBounds(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(1, 1)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if _, err := f.svc.Set(ctx, 2, 10, week, 6); err != nil {
		t.Fatalf("set source: %v", err)
	}

	for _, weeks := range []int{0, -1, MaxRepeatWeeks + 1} {
		if _, err := f.svc.Repeat(ctx, RepeatInput{WeekStart: week, Weeks: weeks}); !errors.Is(err, ErrInvalidWeeks) {
			t.Fatalf("%d weeks: %v", weeks, err)
		}
	}
	if len(f.repo.cells) != 1 {
		t.Fatalf("rejected repeats wrote %d cells", len(f.repo.cells))
	}
}

func TestBulkSkipsArchivedProjectsAndImpossibleHours(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		setup       func(f *fixture, ctx context.Context)
		run         func(f *fixture, ctx context.Context) (*Summary, error)
		wantFilled  int
		wantSkipped int
		wantReason  string
		wantCells   int
	}{
		{
			name: "seed onto an archived project",
			setup: func(f *fixture, _ context.Context) {
				archived := now
				f.repo.projects[10].ArchivedAt = &archived
				f.svc.reports = fakeReports{rows: []report.Row{{UserID: 2, ProjectID: 10, Seconds: 40 * 3600}}}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
			},
			wantFilled: 0, wantSkipped: 5, wantReason: SkipArchived, wantCells: 0,
		},
		{
			// 900 hours over five November weeks is 180 a week, past MaxHours.
			name: "seed whose even split exceeds a week",
			setup: func(f *fixture, _ context.Context) {
				f.svc.reports = fakeReports{rows: []report.Row{{UserID: 2, ProjectID: 10, Seconds: 900 * 3600}}}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
			},
			wantFilled: 0, wantSkipped: 5, wantReason: SkipHours, wantCells: 0,
		},
		{
			name: "repeat from an archived source cell",
			setup: func(f *fixture, ctx context.Context) {
				if _, err := f.svc.Set(ctx, 2, 10, week, 6); err != nil {
					t.Fatalf("set source: %v", err)
				}
				archived := now
				f.repo.projects[10].ArchivedAt = &archived
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Repeat(ctx, RepeatInput{WeekStart: week, Weeks: 2})
			},
			wantFilled: 0, wantSkipped: 2, wantReason: SkipArchived, wantCells: 1,
		},
		{
			name: "seed onto a project outside the person's teams",
			setup: func(f *fixture, _ context.Context) {
				// Cid is in Design only; project 10 is Platform.
				f.svc.reports = fakeReports{rows: []report.Row{{UserID: 3, ProjectID: 10, Seconds: 40 * 3600}}}
			},
			run: func(f *fixture, _ context.Context) (*Summary, error) {
				return f.svc.Seed(adminCtx(9), SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
			},
			wantFilled: 0, wantSkipped: 5, wantReason: SkipOtherTeam, wantCells: 0,
		},
		{
			name: "repeat from a cross-team source cell",
			setup: func(f *fixture, _ context.Context) {
				// Written before the rule existed, so set it behind the service.
				if err := f.repo.Upsert(context.Background(), &Plan{
					WorkspaceID: 1, UserID: 3, ProjectID: 10, WeekStart: week, Hours: 6,
				}); err != nil {
					t.Fatalf("seed source: %v", err)
				}
			},
			run: func(f *fixture, _ context.Context) (*Summary, error) {
				return f.svc.Repeat(adminCtx(9), RepeatInput{WeekStart: week, Weeks: 2})
			},
			wantFilled: 0, wantSkipped: 2, wantReason: SkipOtherTeam, wantCells: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(now)
			ctx := leadCtx(1, 1)
			tc.setup(f, ctx)
			events := f.audit.events

			sum, err := tc.run(f, ctx)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if sum.Filled != tc.wantFilled || sum.Skipped != tc.wantSkipped {
				t.Fatalf("summary = %+v, want filled %d skipped %d", sum, tc.wantFilled, tc.wantSkipped)
			}
			if len(sum.Skips) != tc.wantSkipped {
				t.Fatalf("skips = %+v, want %d", sum.Skips, tc.wantSkipped)
			}
			for _, skip := range sum.Skips {
				if skip.Reason != tc.wantReason {
					t.Fatalf("skip %s = %q, want %q", skip.WeekStart.Format("2006-01-02"), skip.Reason, tc.wantReason)
				}
			}
			if len(sum.Cells) != 0 {
				t.Fatalf("reported cells = %+v", sum.Cells)
			}
			if len(f.repo.cells) != tc.wantCells {
				t.Fatalf("stored %d cells, want %d", len(f.repo.cells), tc.wantCells)
			}
			if f.audit.events != events {
				t.Fatalf("bulk writes audited %d events", f.audit.events-events)
			}
		})
	}
}

// Every skip of a bulk says why, so a caller can explain a zero-fill run.
func TestBulkSkipsSayWhy(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	past := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		as          func() context.Context
		setup       func(f *fixture, ctx context.Context)
		run         func(f *fixture, ctx context.Context) (*Summary, error)
		wantFilled  int
		wantSkipped int
		wantReason  string
	}{
		{
			name: "repeat onto a week that already has hours",
			setup: func(f *fixture, ctx context.Context) {
				for _, w := range []time.Time{week, week.AddDate(0, 0, 7)} {
					if _, err := f.svc.Set(ctx, 2, 10, w, 6); err != nil {
						t.Fatalf("set %s: %v", w.Format("2006-01-02"), err)
					}
				}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Repeat(ctx, RepeatInput{WeekStart: week, Weeks: 2})
			},
			wantFilled: 1, wantSkipped: 1, wantReason: SkipOccupied,
		},
		{
			name: "repeat into weeks that have ended",
			setup: func(f *fixture, _ context.Context) {
				if err := f.repo.Upsert(context.Background(), &Plan{
					WorkspaceID: 1, UserID: 2, ProjectID: 10, WeekStart: past, Hours: 6,
				}); err != nil {
					t.Fatalf("seed source: %v", err)
				}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Repeat(ctx, RepeatInput{WeekStart: past, Weeks: 2})
			},
			wantFilled: 0, wantSkipped: 2, wantReason: SkipFrozen,
		},
		{
			name: "seed for a deactivated person",
			setup: func(f *fixture, _ context.Context) {
				f.svc.reports = fakeReports{rows: []report.Row{{UserID: 4, ProjectID: 10, Seconds: 40 * 3600}}}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
			},
			wantFilled: 0, wantSkipped: 5, wantReason: SkipInactive,
		},
		{
			name: "seed planned by a lead of another team",
			// Ana leads Design only; Bo is in Platform.
			as: func() context.Context { return leadCtx(1, 2) },
			setup: func(f *fixture, _ context.Context) {
				f.svc.reports = fakeReports{rows: []report.Row{{UserID: 2, ProjectID: 10, Seconds: 40 * 3600}}}
			},
			run: func(f *fixture, ctx context.Context) (*Summary, error) {
				return f.svc.Seed(ctx, SeedInput{FromMonth: "2026-10", IntoMonth: "2026-11"})
			},
			wantFilled: 0, wantSkipped: 5, wantReason: SkipForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(now)
			ctx := leadCtx(1, 1)
			if tc.as != nil {
				ctx = tc.as()
			}
			tc.setup(f, ctx)

			sum, err := tc.run(f, ctx)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if sum.Filled != tc.wantFilled || sum.Skipped != tc.wantSkipped {
				t.Fatalf("summary = %+v, want filled %d skipped %d", sum, tc.wantFilled, tc.wantSkipped)
			}
			if len(sum.Skips) != tc.wantSkipped {
				t.Fatalf("skips = %+v, want %d", sum.Skips, tc.wantSkipped)
			}
			for _, skip := range sum.Skips {
				if skip.Reason != tc.wantReason {
					t.Fatalf("skip %s = %q, want %q", skip.WeekStart.Format("2006-01-02"), skip.Reason, tc.wantReason)
				}
				if skip.UserID == 0 || skip.ProjectID == 0 || skip.WeekStart.IsZero() {
					t.Fatalf("skip %+v is missing the cell it names", skip)
				}
			}
		})
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

func TestVacationHoursCountWorkingDaysAndPartialEdges(t *testing.T) {
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // Monday
	// Wednesday to Sunday, 4 hours on Wednesday: Thursday and Friday are full
	// days, Saturday and Sunday never count.
	got := VacationHours(week, 40, []Vacation{{
		StartDate:  time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		StartHours: 4,
	}})
	if got != 20 {
		t.Fatalf("VacationHours = %v, want 20", got)
	}
	// Monday to Friday with a partial first and last day.
	got = VacationHours(week, 40, []Vacation{{
		StartDate: week, EndDate: week.AddDate(0, 0, 4), StartHours: 6, EndHours: 4,
	}})
	if got != 34 {
		t.Fatalf("partial-edge VacationHours = %v, want 34", got)
	}
	if got := VacationHours(week, 40, []Vacation{{
		StartDate: week.AddDate(0, 0, 5), EndDate: week.AddDate(0, 0, 6), StartHours: 8,
	}}); got != 0 {
		t.Fatalf("weekend-only VacationHours = %v, want 0", got)
	}
	if got := VacationHours(week, 40, []Vacation{{
		StartDate: week.AddDate(0, 0, 1), EndDate: week.AddDate(0, 0, 1), StartHours: 4,
	}}); got != 4 {
		t.Fatalf("one-day VacationHours = %v, want 4", got)
	}
	if got := WeeklyCapacity(40, 40); got != 0 {
		t.Fatalf("full week off capacity = %d, want 0", got)
	}
	if got := WeeklyCapacity(40, 20); got != 20 {
		t.Fatalf("half week off capacity = %d, want 20", got)
	}
	if got := WeeklyCapacity(37, 22.2); got != 15 {
		t.Fatalf("rounded capacity = %d, want 15", got)
	}
}

func TestListReducesCapacityByVacation(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	f.repo.vacations = []Vacation{{
		ID: 1, UserID: 2,
		StartDate:  week.AddDate(0, 0, 2),
		EndDate:    week.AddDate(0, 0, 4), // Wednesday to Friday
		StartHours: 4, EndHours: 8,
	}}

	board, err := f.svc.List(leadCtx(1, 1), Filter{From: week, To: week.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byUser := map[uint]PersonWeek{}
	for _, pw := range board.PersonWeeks {
		byUser[pw.UserID] = pw
	}
	if got := byUser[2]; got.VacationHours != 20 || got.Capacity != 20 {
		t.Fatalf("Bo's week = %+v, want 20 vacation hours and 20 capacity", got)
	}
	if got := byUser[1]; got.VacationHours != 0 || got.Capacity != 40 {
		t.Fatalf("Ana's week = %+v, want no vacation and 40 hours", got)
	}
}

func TestAddAndDeleteOwnVacation(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(2, 1)

	v, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC),
		StartHours: 4,
		EndHours:   8,
		Note:       "  family trip  ",
	})
	if err != nil {
		t.Fatalf("AddVacation: %v", err)
	}
	if v.UserID != 2 || v.Note != "family trip" || v.StartDate.Format("2006-01-02") != "2026-11-04" ||
		v.StartHours != 4 || v.EndHours != 8 || v.User != "Bo" {
		t.Fatalf("vacation = %+v", v)
	}
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate: v.StartDate, EndDate: v.EndDate, StartHours: 8, EndHours: 8,
	}); !errors.Is(err, ErrVacationOverlap) {
		t.Fatalf("overlap: %v", err)
	}
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate: v.EndDate, EndDate: v.StartDate, StartHours: 8, EndHours: 8,
	}); !errors.Is(err, ErrInvalidVacation) {
		t.Fatalf("backwards range: %v", err)
	}
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		StartHours: 9,
	}); !errors.Is(err, ErrInvalidVacation) {
		t.Fatalf("more than one working day: %v", err)
	}
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		StartHours: 4.25,
	}); !errors.Is(err, ErrInvalidVacation) {
		t.Fatalf("more than one decimal: %v", err)
	}
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2027, 11, 3, 0, 0, 0, 0, time.UTC),
		StartHours: 8, EndHours: 8,
	}); !errors.Is(err, ErrInvalidVacation) {
		t.Fatalf("longer than the cap: %v", err)
	}

	if err := f.svc.DeleteVacation(leadCtx(1, 1), v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted another user's vacation: %v", err)
	}
	if err := f.svc.DeleteVacation(ctx, v.ID); err != nil {
		t.Fatalf("DeleteVacation: %v", err)
	}
	vacations, err := f.svc.ListVacations(ctx, VacationQuery{})
	if err != nil || len(vacations) != 0 {
		t.Fatalf("after delete: %v, %+v", err, vacations)
	}

	// 2026-11-07 is a Saturday: a weekend edge is stored as zero hours.
	weekend, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
		StartHours: 8,
		EndHours:   8,
	})
	if err != nil {
		t.Fatalf("weekend start: %v", err)
	}
	if weekend.StartHours != 0 || weekend.EndHours != 8 {
		t.Fatalf("weekend start edges = %+v, want 0 and 8", weekend)
	}
}

func TestVacationHoursClampsStoredEdgesAndTotalHoursSumsTheRange(t *testing.T) {
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // Monday
	// A vacation stored as 8 hours on a day that is now 4 hours (a 20-hour
	// week) removes 4, so lowering someone's hours cannot over-remove.
	got := VacationHours(week, 20, []Vacation{{
		StartDate: week.AddDate(0, 0, 2), EndDate: week.AddDate(0, 0, 2), StartHours: 8,
	}})
	if got != 4 {
		t.Fatalf("clamped VacationHours = %v, want 4", got)
	}
	// Wednesday to the next Tuesday is six working days, weekends excluded.
	if got := VacationTotalHours(Vacation{
		StartDate: week.AddDate(0, 0, 2), EndDate: week.AddDate(0, 0, 8), StartHours: 8, EndHours: 8,
	}, 40); got != 40 {
		t.Fatalf("VacationTotalHours = %v, want 40", got)
	}
}

func TestLeadReadsTeamVacationsOnly(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f := newFixture(now)
	ctx := leadCtx(2, 1)
	if _, err := f.svc.AddVacation(ctx, VacationInput{
		StartDate:  time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		StartHours: 8,
	}); err != nil {
		t.Fatalf("AddVacation: %v", err)
	}

	// Ana leads Platform, which Bo belongs to.
	vacations, err := f.svc.ListVacations(leadCtx(1, 1), VacationQuery{UserID: 2})
	if err != nil || len(vacations) != 1 || vacations[0].User != "Bo" {
		t.Fatalf("lead read: %v, %+v", err, vacations)
	}
	// Cid leads Design, not Platform.
	if _, err := f.svc.ListVacations(leadCtx(3, 2), VacationQuery{UserID: 2}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("foreign lead read: %v", err)
	}
}
