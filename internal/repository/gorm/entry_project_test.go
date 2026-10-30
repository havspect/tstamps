package gorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/project"
)

func TestEntryRepositoryLifecycle(t *testing.T) {
	db := newTestDB(t)
	entryRepo := NewEntryRepository(db)
	projectRepo := NewProjectRepository(db)
	txRunner := NewTxRunner(db)

	ctx := context.Background()

	// Seed workspace
	ws := &Workspace{Slug: "acme"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsCtx := WithWorkspaceID(ctx, ws.ID)

	// Seed user & team
	user := &User{Email: "user@example.com", Name: "User", PasswordHash: "x"}
	if err := db.WithContext(wsCtx).Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	team := &Team{Name: "Platform"}
	if err := db.WithContext(wsCtx).Create(team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}

	// Create project
	proj := &project.Project{
		WorkspaceID: ws.ID,
		TeamID:      team.ID,
		Name:        "API Backend",
		Color:       "#4a90e2",
	}
	if err := projectRepo.Create(wsCtx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// 1. IsProjectLoggable
	loggable, err := entryRepo.IsProjectLoggable(wsCtx, proj.ID)
	if err != nil || !loggable {
		t.Fatalf("IsProjectLoggable: got (%v, %v), want (true, nil)", loggable, err)
	}

	// 2. Create running entry
	now := time.Now().UTC().Truncate(time.Second)
	e1 := &entry.Entry{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		ProjectID:   proj.ID,
		StartAt:     now,
		Timezone:    "UTC",
		Note:        "Working on backend",
	}
	if err := entryRepo.Create(wsCtx, e1); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if e1.ID == 0 {
		t.Fatal("expected non-zero entry ID after create")
	}

	// 3. Duplicate running entry triggers ErrRunningExists
	e2 := &entry.Entry{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		ProjectID:   proj.ID,
		StartAt:     now.Add(time.Minute),
		Timezone:    "UTC",
	}
	err = entryRepo.Create(wsCtx, e2)
	if !errors.Is(err, entry.ErrRunningExists) {
		t.Fatalf("second running entry: got %v, want ErrRunningExists", err)
	}

	// 4. GetRunning
	running, err := entryRepo.GetRunning(wsCtx, user.ID)
	if err != nil || running == nil {
		t.Fatalf("GetRunning: got (%+v, %v)", running, err)
	}
	if running.ID != e1.ID {
		t.Errorf("running ID = %d, want %d", running.ID, e1.ID)
	}

	// 5. Update entry (stop timer)
	end := now.Add(2 * time.Hour)
	running.EndAt = &end
	running.Note = "Finished backend"
	running.UpdatedAt = end
	if err := entryRepo.Update(wsCtx, running); err != nil {
		t.Fatalf("Update entry: %v", err)
	}

	// 6. GetRunning should now return ErrNotFound
	_, err = entryRepo.GetRunning(wsCtx, user.ID)
	if !errors.Is(err, entry.ErrNotFound) {
		t.Fatalf("GetRunning after stop: got %v, want ErrNotFound", err)
	}

	// 7. List entries
	list, err := entryRepo.List(wsCtx, entry.ListFilter{UserID: user.ID})
	if err != nil {
		t.Fatalf("List entries: %v", err)
	}
	if len(list) != 1 || list[0].ID != e1.ID {
		t.Fatalf("List result mismatch: %+v", list)
	}

	// 8. SoftDelete
	delAt := now.Add(3 * time.Hour)
	if err := entryRepo.SoftDelete(wsCtx, e1.ID, delAt); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	// Listing without deleted should return 0
	listAfterDel, err := entryRepo.List(wsCtx, entry.ListFilter{UserID: user.ID, IncludeDeleted: false})
	if err != nil || len(listAfterDel) != 0 {
		t.Fatalf("List after soft delete: len=%d, err=%v", len(listAfterDel), err)
	}
	// Listing with IncludeDeleted returns 1
	listWithDel, err := entryRepo.List(wsCtx, entry.ListFilter{UserID: user.ID, IncludeDeleted: true})
	if err != nil || len(listWithDel) != 1 {
		t.Fatalf("List with includeDeleted: len=%d, err=%v", len(listWithDel), err)
	}

	// 9. Restore
	if err := entryRepo.Restore(wsCtx, e1.ID); err != nil {
		t.Fatalf("Restore entry: %v", err)
	}
	restored, err := entryRepo.GetByID(wsCtx, e1.ID)
	if err != nil || restored.DeletedAt != nil {
		t.Fatalf("GetByID restored: got %+v, %v", restored, err)
	}

	// 10. TxRunner rollback
	err = txRunner.Run(wsCtx, func(txCtx context.Context) error {
		txEntry := &entry.Entry{
			WorkspaceID: ws.ID,
			UserID:      user.ID,
			ProjectID:   proj.ID,
			StartAt:     now.Add(10 * time.Hour),
			EndAt:       &end,
		}
		if err := entryRepo.Create(txCtx, txEntry); err != nil {
			return err
		}
		return errors.New("abort transaction")
	})
	if err == nil {
		t.Fatal("expected error from aborted transaction")
	}
	listAfterAbort, _ := entryRepo.List(wsCtx, entry.ListFilter{UserID: user.ID})
	if len(listAfterAbort) != 1 {
		t.Errorf("expected 1 entry after transaction rollback, got %d", len(listAfterAbort))
	}
}

func TestProjectRepositoryLifecycle(t *testing.T) {
	db := newTestDB(t)
	projectRepo := NewProjectRepository(db)
	entryRepo := NewEntryRepository(db)

	ctx := context.Background()
	ws := &Workspace{Slug: "corp"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsCtx := WithWorkspaceID(ctx, ws.ID)

	team1 := &Team{Name: "Backend"}
	team2 := &Team{Name: "Frontend"}
	archivedTeam := &Team{Name: "OldTeam", ArchivedAt: &time.Time{}}
	if err := db.WithContext(wsCtx).Create(team1).Error; err != nil {
		t.Fatalf("create team1: %v", err)
	}
	if err := db.WithContext(wsCtx).Create(team2).Error; err != nil {
		t.Fatalf("create team2: %v", err)
	}
	if err := db.WithContext(wsCtx).Create(archivedTeam).Error; err != nil {
		t.Fatalf("create archivedTeam: %v", err)
	}

	// 1. IsTeamActive
	active, err := projectRepo.IsTeamActive(wsCtx, team1.ID)
	if err != nil || !active {
		t.Fatalf("team1 IsTeamActive: got (%v, %v), want (true, nil)", active, err)
	}
	activeOld, err := projectRepo.IsTeamActive(wsCtx, archivedTeam.ID)
	if err != nil || activeOld {
		t.Fatalf("archivedTeam IsTeamActive: got (%v, %v), want (false, nil)", activeOld, err)
	}

	// 2. Create Project
	p := &project.Project{
		WorkspaceID: ws.ID,
		TeamID:      team1.ID,
		Name:        "Service Mesh",
		Color:       "#00ff00",
	}
	if err := projectRepo.Create(wsCtx, p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected project ID to be assigned")
	}

	// 3. HasEntries == false initially
	hasEntries, err := projectRepo.HasEntries(wsCtx, p.ID)
	if err != nil || hasEntries {
		t.Fatalf("HasEntries before entries: got (%v, %v), want (false, nil)", hasEntries, err)
	}

	// 4. Add an entry to project and re-check HasEntries
	user := &User{Email: "dev@corp.com", Name: "Dev", PasswordHash: "p"}
	db.WithContext(wsCtx).Create(user)
	e := &entry.Entry{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		ProjectID:   p.ID,
		StartAt:     time.Now(),
		Timezone:    "UTC",
	}
	if err := entryRepo.Create(wsCtx, e); err != nil {
		t.Fatalf("create entry on project: %v", err)
	}
	hasEntries, err = projectRepo.HasEntries(wsCtx, p.ID)
	if err != nil || !hasEntries {
		t.Fatalf("HasEntries after entry: got (%v, %v), want (true, nil)", hasEntries, err)
	}

	// 5. Update project
	p.Name = "Service Mesh V2"
	p.Color = "#ff0000"
	p.UpdatedAt = time.Now()
	if err := projectRepo.Update(wsCtx, p); err != nil {
		t.Fatalf("Update project: %v", err)
	}
	updated, err := projectRepo.GetByID(wsCtx, p.ID)
	if err != nil || updated.Name != "Service Mesh V2" {
		t.Fatalf("GetByID mismatch: %+v, %v", updated, err)
	}

	// 6. Archive project
	archiveAt := time.Now()
	if err := projectRepo.Archive(wsCtx, p.ID, archiveAt); err != nil {
		t.Fatalf("Archive project: %v", err)
	}
	archived, err := projectRepo.GetByID(wsCtx, p.ID)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("expected project to be archived: %+v", archived)
	}

	// List without archived
	activeList, err := projectRepo.List(wsCtx, project.ListFilter{IncludeArchived: false})
	if err != nil || len(activeList) != 0 {
		t.Fatalf("active projects list should be empty: len=%d", len(activeList))
	}
	allList, err := projectRepo.List(wsCtx, project.ListFilter{IncludeArchived: true})
	if err != nil || len(allList) != 1 {
		t.Fatalf("all projects list should have 1: len=%d", len(allList))
	}

	// 7. Restore project
	if err := projectRepo.Restore(wsCtx, p.ID); err != nil {
		t.Fatalf("Restore project: %v", err)
	}
	restored, err := projectRepo.GetByID(wsCtx, p.ID)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("expected project to be restored: %+v", restored)
	}
}

func TestEntryProjectAuditRecorder(t *testing.T) {
	db := newTestDB(t)
	auditRepo := NewAuditRepository(db)
	adapter := NewAuditRecorderAdapter(auditRepo)

	ctx := context.Background()
	ws := &Workspace{Slug: "audit-test"}
	db.WithContext(ctx).Create(ws)
	wsCtx := WithWorkspaceID(ctx, ws.ID)

	principal := &auth.Principal{
		WorkspaceID: ws.ID,
		UserID:      42,
		Roles:       []string{"admin"},
	}
	actorCtx := auth.WithPrincipal(wsCtx, principal)

	err := adapter.Record(actorCtx, "project.create", "project", 1, map[string]any{
		"name": "New Project",
	})
	if err != nil {
		t.Fatalf("Record audit: %v", err)
	}

	var logs []AuditLog
	if err := db.WithContext(wsCtx).Find(&logs).Error; err != nil {
		t.Fatalf("query audit_log: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].Action != "project.create" || logs[0].ActorUserID != 42 || logs[0].ActorRole != "admin" {
		t.Errorf("audit log mismatch: %+v", logs[0])
	}
}

func TestTenancyIsolation(t *testing.T) {
	db := newTestDB(t)
	entryRepo := NewEntryRepository(db)

	ctx := context.Background()
	ws1 := &Workspace{Slug: "ws1"}
	ws2 := &Workspace{Slug: "ws2"}
	db.WithContext(ctx).Create(ws1)
	db.WithContext(ctx).Create(ws2)

	ws1Ctx := WithWorkspaceID(ctx, ws1.ID)
	ws2Ctx := WithWorkspaceID(ctx, ws2.ID)

	// User in WS1
	u1 := &User{Email: "u@ws1.com", Name: "U1", PasswordHash: "p"}
	db.WithContext(ws1Ctx).Create(u1)
	p1 := &Project{TeamID: 1, Name: "P1"}
	db.WithContext(ws1Ctx).Create(p1)

	e1 := &entry.Entry{
		WorkspaceID: ws1.ID,
		UserID:      u1.ID,
		ProjectID:   p1.ID,
		StartAt:     time.Now(),
		Timezone:    "UTC",
	}
	if err := entryRepo.Create(ws1Ctx, e1); err != nil {
		t.Fatalf("create entry in ws1: %v", err)
	}

	// Querying from WS2 should not find WS1 entry
	_, err := entryRepo.GetByID(ws2Ctx, e1.ID)
	if !errors.Is(err, entry.ErrNotFound) {
		t.Fatalf("ws2 reading ws1 entry: got %v, want ErrNotFound", err)
	}

	listWS2, err := entryRepo.List(ws2Ctx, entry.ListFilter{})
	if err != nil || len(listWS2) != 0 {
		t.Fatalf("ws2 listing ws1 entries: len=%d, err=%v", len(listWS2), err)
	}
}

func TestEntryListOverlapWindow(t *testing.T) {
	db := newTestDB(t)
	repo := NewEntryRepository(db)
	ctx := context.Background()

	ws := &Workspace{Slug: "overlap"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsCtx := WithWorkspaceID(ctx, ws.ID)
	user := &User{Email: "u@example.com", Name: "U", PasswordHash: "x"}
	if err := db.WithContext(wsCtx).Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	team := &Team{Name: "T"}
	if err := db.WithContext(wsCtx).Create(team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	proj := &project.Project{WorkspaceID: ws.ID, TeamID: team.ID, Name: "P", Color: "#000"}
	if err := NewProjectRepository(db).Create(wsCtx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	base := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	end := base.Add(2 * time.Hour)
	live := &entry.Entry{WorkspaceID: ws.ID, UserID: user.ID, ProjectID: proj.ID, StartAt: base, EndAt: &end}
	running := &entry.Entry{WorkspaceID: ws.ID, UserID: user.ID, ProjectID: proj.ID, StartAt: base.Add(3 * time.Hour)}
	deletedAt := base.Add(5 * time.Hour)
	deleted := &entry.Entry{WorkspaceID: ws.ID, UserID: user.ID, ProjectID: proj.ID, StartAt: base.Add(4 * time.Hour), EndAt: &deletedAt}
	for _, e := range []*entry.Entry{live, running, deleted} {
		if err := repo.Create(wsCtx, e); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}
	del := base.Add(6 * time.Hour)
	if err := repo.SoftDelete(wsCtx, deleted.ID, del); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	from, to := base.Add(time.Hour), base.Add(90*time.Minute)
	got, err := repo.List(wsCtx, entry.ListFilter{UserID: user.ID, ProjectID: proj.ID, From: &from, To: &to})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != live.ID {
		t.Fatalf("overlap window: got %+v, want only the live entry", got)
	}

	from = base.Add(2 * time.Hour)
	got, err = repo.List(wsCtx, entry.ListFilter{UserID: user.ID, ProjectID: proj.ID, From: &from})
	if err != nil {
		t.Fatalf("list open-ended: %v", err)
	}
	if len(got) != 1 || got[0].ID != running.ID {
		t.Fatalf("open-ended window: got %+v, want the running entry only", got)
	}
}
