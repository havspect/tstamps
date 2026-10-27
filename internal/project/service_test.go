package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
)

var testNow = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeUOW struct{}

func (u *fakeUOW) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type recordedAudit struct {
	action     string
	targetType string
	targetID   uint
	changes    any
}

type fakeAudit struct {
	records []recordedAudit
}

func (a *fakeAudit) Record(ctx context.Context, action string, targetType string, targetID uint, changes any) error {
	a.records = append(a.records, recordedAudit{
		action:     action,
		targetType: targetType,
		targetID:   targetID,
		changes:    changes,
	})
	return nil
}

type fakeTeams struct {
	active map[uint]bool
}

func (t *fakeTeams) IsTeamActive(ctx context.Context, teamID uint) (bool, error) {
	return t.active[teamID], nil
}

type fakeRepo struct {
	projects map[uint]*Project
	entries  map[uint]bool // projectID -> hasEntries
	nextID   uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		projects: make(map[uint]*Project),
		entries:  make(map[uint]bool),
		nextID:   1,
	}
}

func (r *fakeRepo) GetByID(ctx context.Context, id uint) (*Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakeRepo) List(ctx context.Context, filter ListFilter) ([]Project, error) {
	var out []Project
	for _, p := range r.projects {
		if !filter.IncludeArchived && p.ArchivedAt != nil {
			continue
		}
		if filter.TeamID != 0 && p.TeamID != filter.TeamID {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (r *fakeRepo) Create(ctx context.Context, p *Project) error {
	p.ID = r.nextID
	r.nextID++
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(ctx context.Context, p *Project) error {
	if _, ok := r.projects[p.ID]; !ok {
		return ErrNotFound
	}
	cp := *p
	r.projects[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Archive(ctx context.Context, id uint, at time.Time) error {
	p, ok := r.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.ArchivedAt = &at
	p.UpdatedAt = at
	return nil
}

func (r *fakeRepo) Restore(ctx context.Context, id uint) error {
	p, ok := r.projects[id]
	if !ok {
		return ErrNotFound
	}
	p.ArchivedAt = nil
	p.UpdatedAt = testNow
	return nil
}

func (r *fakeRepo) HasEntries(ctx context.Context, id uint) (bool, error) {
	return r.entries[id], nil
}

func testContext(p *auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), p)
}

func newTestService() (*Service, *fakeRepo, *fakeAudit, *fakeTeams) {
	repo := newFakeRepo()
	audit := &fakeAudit{}
	teams := &fakeTeams{
		active: map[uint]bool{1: true, 2: true, 99: false},
	}
	svc := NewService(
		repo,
		&fakeUOW{},
		audit,
		teams,
		authz.NewPolicyEngine(),
		fixedClock{now: testNow},
	)
	return svc, repo, audit, teams
}

func TestCreateProject(t *testing.T) {
	svc, repo, audit, _ := newTestService()
	admin := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{WorkspaceID: 1, UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{1}}
	member := &auth.Principal{WorkspaceID: 1, UserID: 3, Roles: []string{"member"}}

	// 1. Regular member cannot create project
	_, err := svc.CreateProject(testContext(member), 1, "Alpha", "#fff")
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("member CreateProject: got %v, want ErrForbidden", err)
	}

	// 2. Lead cannot create project in foreign team
	_, err = svc.CreateProject(testContext(lead), 2, "Beta", "#000")
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("lead foreign team CreateProject: got %v, want ErrForbidden", err)
	}

	// 3. Lead can create in led team
	proj, err := svc.CreateProject(testContext(lead), 1, "Alpha", "#123")
	if err != nil {
		t.Fatalf("lead CreateProject: %v", err)
	}
	if proj.ID == 0 || proj.TeamID != 1 || proj.Name != "Alpha" {
		t.Errorf("proj fields mismatch: %+v", proj)
	}
	if len(audit.records) != 1 || audit.records[0].action != "project.create" {
		t.Errorf("audit records mismatch: %+v", audit.records)
	}

	// 4. Inactive team fails
	_, err = svc.CreateProject(testContext(admin), 99, "Inactive", "#000")
	if !errors.Is(err, ErrTeamNotActive) {
		t.Fatalf("inactive team CreateProject: got %v, want ErrTeamNotActive", err)
	}

	// 5. Empty name fails
	_, err = svc.CreateProject(testContext(admin), 1, "   ", "#000")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("empty name CreateProject: got %v, want ErrInvalidName", err)
	}
	_ = repo
}

func TestUpdateProject(t *testing.T) {
	svc, repo, audit, _ := newTestService()
	admin := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{WorkspaceID: 1, UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{1}}

	p := &Project{WorkspaceID: 1, TeamID: 1, Name: "Initial", Color: "#fff"}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 1. Name & color update
	newName := "Renamed"
	newColor := "#000"
	updated, err := svc.UpdateProject(testContext(lead), p.ID, nil, &newName, &newColor, nil)
	if err != nil {
		t.Fatalf("update name/color: %v", err)
	}
	if updated.Name != "Renamed" || updated.Color != "#000" {
		t.Errorf("updated mismatch: %+v", updated)
	}

	// 2. Change team when HasEntries == true fails with ErrProjectHasEntries
	repo.entries[p.ID] = true
	destTeam := uint(2)
	_, err = svc.UpdateProject(testContext(admin), p.ID, &destTeam, nil, nil, nil)
	if !errors.Is(err, ErrProjectHasEntries) {
		t.Fatalf("change team with entries: got %v, want ErrProjectHasEntries", err)
	}

	// 3. Change team when HasEntries == false succeeds
	repo.entries[p.ID] = false
	updated, err = svc.UpdateProject(testContext(admin), p.ID, &destTeam, nil, nil, nil)
	if err != nil {
		t.Fatalf("change team without entries: %v", err)
	}
	if updated.TeamID != 2 {
		t.Errorf("teamID not updated: got %d, want 2", updated.TeamID)
	}

	// 4. Change team to inactive team fails
	inactiveTeam := uint(99)
	_, err = svc.UpdateProject(testContext(admin), p.ID, &inactiveTeam, nil, nil, nil)
	if !errors.Is(err, ErrTeamNotActive) {
		t.Fatalf("change team to inactive: got %v, want ErrTeamNotActive", err)
	}

	if len(audit.records) == 0 {
		t.Error("expected audit records for updates")
	}
}

func TestArchiveAndRestoreProject(t *testing.T) {
	svc, repo, audit, teams := newTestService()
	admin := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"admin"}}
	member := &auth.Principal{WorkspaceID: 1, UserID: 3, Roles: []string{"member"}}

	p := &Project{WorkspaceID: 1, TeamID: 1, Name: "Test", Color: "#fff"}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 1. Regular member cannot archive
	err := svc.ArchiveProject(testContext(member), p.ID)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("member Archive: got %v, want ErrForbidden", err)
	}

	// 2. Admin archives project
	err = svc.ArchiveProject(testContext(admin), p.ID)
	if err != nil {
		t.Fatalf("admin Archive: %v", err)
	}
	stored, _ := repo.GetByID(context.Background(), p.ID)
	if stored.ArchivedAt == nil {
		t.Fatal("expected project to be archived")
	}

	// 3. Regular member cannot restore
	_, err = svc.RestoreProject(testContext(member), p.ID)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("member Restore: got %v, want ErrForbidden", err)
	}

	// 4. Restore fails if team is inactive
	teams.active[1] = false
	_, err = svc.RestoreProject(testContext(admin), p.ID)
	if !errors.Is(err, ErrTeamNotActive) {
		t.Fatalf("restore with inactive team: got %v, want ErrTeamNotActive", err)
	}

	// 5. Restore succeeds when team is active
	teams.active[1] = true
	restored, err := svc.RestoreProject(testContext(admin), p.ID)
	if err != nil {
		t.Fatalf("admin Restore: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Fatal("expected restored project to have nil ArchivedAt")
	}

	// Verify audits
	var archiveFound, restoreFound bool
	for _, rec := range audit.records {
		if rec.action == "project.archive" && rec.targetID == p.ID {
			archiveFound = true
		}
		if rec.action == "project.restore" && rec.targetID == p.ID {
			restoreFound = true
		}
	}
	if !archiveFound || !restoreFound {
		t.Errorf("missing audit logs: archive=%v, restore=%v", archiveFound, restoreFound)
	}
}

func TestListAndGetByID(t *testing.T) {
	svc, repo, _, _ := newTestService()
	admin := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"admin"}}
	member := &auth.Principal{WorkspaceID: 1, UserID: 3, Roles: []string{"member"}}

	archivedTime := testNow
	p1 := &Project{WorkspaceID: 1, TeamID: 1, Name: "Active"}
	p2 := &Project{WorkspaceID: 1, TeamID: 1, Name: "Archived", ArchivedAt: &archivedTime}
	repo.Create(context.Background(), p1)
	repo.Create(context.Background(), p2)

	// Member lists active projects
	projects, err := svc.ListProjects(testContext(member), false)
	if err != nil {
		t.Fatalf("member ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "Active" {
		t.Errorf("member ListProjects mismatch: %+v", projects)
	}

	// Member cannot list with includeArchived=true
	_, err = svc.ListProjects(testContext(member), true)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("member includeArchived: got %v, want ErrForbidden", err)
	}

	// Admin lists all
	adminProjects, err := svc.ListProjects(testContext(admin), true)
	if err != nil {
		t.Fatalf("admin ListProjects: %v", err)
	}
	if len(adminProjects) != 2 {
		t.Errorf("admin ListProjects mismatch: %+v", adminProjects)
	}

	// Member viewing active project succeeds
	got, err := svc.GetByID(testContext(member), p1.ID)
	if err != nil || got.ID != p1.ID {
		t.Fatalf("GetByID active: got (%+v, %v)", got, err)
	}

	// Member viewing archived project is forbidden
	_, err = svc.GetByID(testContext(member), p2.ID)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("GetByID archived member: got %v, want ErrForbidden", err)
	}

	// Admin viewing archived project succeeds
	gotArchived, err := svc.GetByID(testContext(admin), p2.ID)
	if err != nil || gotArchived.ID != p2.ID {
		t.Fatalf("admin GetByID archived: got (%+v, %v)", gotArchived, err)
	}
}
