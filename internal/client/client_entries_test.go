package client

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/project"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"github.com/havspect/tstamps/internal/team"
)

// seed starts the API over a store with an admin, a team and a project, and
// returns a bearer client plus the project id.
func seed(t *testing.T) (*Client, uint) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tstamps.db")
	handler, db := buildTestServer(t, dbPath)

	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	onbRepo := gormrepo.NewOnboardingRepository(db)
	ws, err := onbRepo.CreateWorkspace(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	adminID, err := onbRepo.CreateInitialAdmin(context.Background(), ws.ID, "ada@example.com", "Ada Lovelace", hash, "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	ctx := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	teamRepo := gormrepo.NewTeamRepository(db)
	tm := &team.Team{Name: "Platform", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := teamRepo.Create(ctx, tm); err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := teamRepo.ReplaceMemberships(ctx, tm.ID, []team.Membership{{TeamID: tm.ID, UserID: adminID, Role: "lead"}}); err != nil {
		t.Fatalf("replace memberships: %v", err)
	}

	projRepo := gormrepo.NewProjectRepository(db)
	proj := &project.Project{TeamID: tm.ID, Name: "Timeline", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := New(Config{URL: srv.URL})
	if _, err := c.Login("ada@example.com", testPassword); err != nil {
		t.Fatalf("login: %v", err)
	}
	tok, err := c.CreateToken("test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return New(Config{URL: srv.URL, Token: tok.Token}), proj.ID
}

func TestEntriesReportsAndAdminReads(t *testing.T) {
	c, projectID := seed(t)

	start := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	end := start.Add(150 * time.Minute)
	e, err := c.CreateEntry(EntryInput{ProjectID: projectID, StartAt: start, EndAt: end, Timezone: "UTC", Note: "drag"})
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if e.ID == 0 || e.Note != "drag" {
		t.Fatalf("created entry = %+v", e)
	}

	entries, err := c.ListEntries(EntryQuery{From: "2026-10-19", To: "2026-10-21"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != e.ID {
		t.Fatalf("entries = %+v, want the created one", entries)
	}

	note := "snap"
	patched, err := c.PatchEntry(e.ID, EntryPatch{Note: &note})
	if err != nil {
		t.Fatalf("patch entry: %v", err)
	}
	if patched.Note != "snap" {
		t.Fatalf("patched note %q, want snap", patched.Note)
	}

	report, err := c.Report(ReportQuery{From: "2026-10-19", To: "2026-10-26", By: "week"})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.TotalSeconds != 9000 {
		t.Fatalf("report total %d, want 9000", report.TotalSeconds)
	}
	if len(report.Rows) != 1 || report.Rows[0].Project != "Timeline" || report.Rows[0].Team != "Platform" {
		t.Fatalf("report rows = %+v", report.Rows)
	}

	csv, err := c.ReportCSV(ReportQuery{From: "2026-10-19", To: "2026-10-26"})
	if err != nil {
		t.Fatalf("report csv: %v", err)
	}
	if !strings.Contains(string(csv), ",9000,snap") {
		t.Fatalf("csv does not carry the entry:\n%s", csv)
	}

	if err := c.DeleteEntry(e.ID); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	live, err := c.ListEntries(EntryQuery{From: "2026-10-19", To: "2026-10-21"})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("deleted entry still listed: %+v", live)
	}
	all, err := c.ListEntries(EntryQuery{From: "2026-10-19", To: "2026-10-21", IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list deleted: %v", err)
	}
	if len(all) != 1 || all[0].DeletedAt == nil {
		t.Fatalf("admin list = %+v, want the soft-deleted entry", all)
	}

	projects, err := c.ListProjects(false)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != projectID {
		t.Fatalf("projects = %+v", projects)
	}
	teams, err := c.ListTeams()
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 1 || teams[0].Name != "Platform" {
		t.Fatalf("teams = %+v", teams)
	}
	users, err := c.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || !users[0].IsAdmin {
		t.Fatalf("users = %+v", users)
	}
}

func TestTimerClient(t *testing.T) {
	c, projectID := seed(t)

	if e, err := c.Timer(); err != nil || e != nil {
		t.Fatalf("timer before start = %+v, %v", e, err)
	}
	running, err := c.StartTimer(projectID, "UTC", "OAuth")
	if err != nil {
		t.Fatalf("start timer: %v", err)
	}
	if running.EndAt != nil || running.Note != "OAuth" {
		t.Fatalf("running entry = %+v", running)
	}
	if _, err := c.StartTimer(projectID, "UTC", ""); err == nil {
		t.Fatal("second start succeeded, want a 409")
	}
	stopped, err := c.StopTimer("done")
	if err != nil {
		t.Fatalf("stop timer: %v", err)
	}
	if stopped.EndAt == nil || stopped.Note != "done" {
		t.Fatalf("stopped entry = %+v", stopped)
	}
}
