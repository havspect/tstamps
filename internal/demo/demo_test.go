package demo

import (
	"context"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"gorm.io/gorm"
)

func seedTestDB(t *testing.T) (*gorm.DB, uint) {
	t.Helper()
	db, err := gormrepo.Open(t.TempDir() + "/demo.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gormrepo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	if err := Seed(context.Background(), db, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var ws gormrepo.Workspace
	if err := db.WithContext(gormrepo.WithBypassTenancy(context.Background())).Where("slug = ?", Slug).First(&ws).Error; err != nil {
		t.Fatalf("find workspace: %v", err)
	}
	return db, ws.ID
}

func TestSeedFillsTheDemoWorkspace(t *testing.T) {
	db, wsID := seedTestDB(t)
	ctx := gormrepo.WithWorkspaceID(context.Background(), wsID)

	count := func(model any) int64 {
		var n int64
		if err := db.WithContext(ctx).Model(model).Count(&n).Error; err != nil {
			t.Fatalf("count %T: %v", model, err)
		}
		return n
	}

	if got := count(&gormrepo.User{}); got != int64(len(people)) {
		t.Errorf("users = %d, want %d", got, len(people))
	}
	if got := count(&gormrepo.Team{}); got != 2 {
		t.Errorf("teams = %d, want 2", got)
	}
	if got := count(&gormrepo.Project{}); got != int64(len(projectDefs)) {
		t.Errorf("projects = %d, want %d", got, len(projectDefs))
	}
	var archived int64
	db.WithContext(ctx).Model(&gormrepo.Project{}).Where("archived_at IS NOT NULL").Count(&archived)
	if archived != 1 {
		t.Errorf("archived projects = %d, want 1", archived)
	}
	if got, want := count(&gormrepo.TeamMembership{}), int64(len(people)); got != want {
		t.Errorf("memberships = %d, want %d", got, want)
	}

	var ada gormrepo.User
	if err := db.WithContext(ctx).Where("email = ?", "ada@example.com").First(&ada).Error; err != nil {
		t.Fatalf("find ada: %v", err)
	}
	if !ada.IsAdmin {
		t.Error("ada is not an admin")
	}
	if !auth.VerifyPassword(ada.PasswordHash, "demo") {
		t.Error("ada cannot sign in with the demo password")
	}
	var leo gormrepo.User
	db.WithContext(ctx).Where("email = ?", "leo@example.com").First(&leo)
	var leoMembership gormrepo.TeamMembership
	if err := db.WithContext(ctx).Where("user_id = ?", leo.ID).First(&leoMembership).Error; err != nil {
		t.Fatalf("leo membership: %v", err)
	}
	if leoMembership.Role != "lead" {
		t.Errorf("leo role = %q, want lead", leoMembership.Role)
	}
	var background gormrepo.User
	db.WithContext(ctx).Where("email = ?", "priya@example.com").First(&background)
	if auth.VerifyPassword(background.PasswordHash, "demo") {
		t.Error("a background user signs in with the public demo password")
	}

	var running int64
	db.WithContext(ctx).Model(&gormrepo.Entry{}).Where("end_at IS NULL").Count(&running)
	if running != 2 {
		t.Errorf("running entries = %d, want 2", running)
	}
	var deleted int64
	db.WithContext(ctx).Model(&gormrepo.Entry{}).Where("deleted_at IS NOT NULL").Count(&deleted)
	if deleted != 1 {
		t.Errorf("deleted entries = %d, want 1", deleted)
	}

	var night gormrepo.Entry
	if err := db.WithContext(ctx).Where("note = ?", "Night deploy").First(&night).Error; err != nil {
		t.Fatalf("find midnight entry: %v", err)
	}
	sydney, _ := time.LoadLocation("Australia/Sydney")
	if s, e := night.StartAt.In(sydney), night.EndAt.In(sydney); s.Day() == e.Day() {
		t.Errorf("night entry does not cross midnight: %s to %s", s, e)
	}

	var clash []gormrepo.Entry
	db.WithContext(ctx).Where("note = ?", "Parallel review").Order("start_at").Find(&clash)
	if len(clash) != 2 || !clash[0].EndAt.After(clash[1].StartAt) {
		t.Errorf("overlapping entries missing or not overlapping: %+v", clash)
	}

	var onArchived int64
	var legacy gormrepo.Project
	if err := db.WithContext(ctx).Where("name = ?", "Legacy import").First(&legacy).Error; err != nil {
		t.Fatalf("find archived project: %v", err)
	}
	db.WithContext(ctx).Model(&gormrepo.Entry{}).Where("project_id = ?", legacy.ID).Count(&onArchived)
	if onArchived == 0 {
		t.Error("archived project has no hours")
	}

	monday := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	var thisWeek int64
	db.WithContext(ctx).Model(&gormrepo.Entry{}).
		Where("start_at >= ? AND deleted_at IS NULL", monday).Count(&thisWeek)
	if thisWeek == 0 {
		t.Error("no entries in the current week")
	}

	if got := count(&gormrepo.Plan{}); got == 0 {
		t.Fatal("no plans seeded")
	}
	var past, future int64
	db.WithContext(ctx).Model(&gormrepo.Plan{}).Where("week_start < ?", monday).Count(&past)
	db.WithContext(ctx).Model(&gormrepo.Plan{}).Where("week_start > ?", monday).Count(&future)
	if past == 0 || future == 0 {
		t.Errorf("plans do not span past and future: past=%d future=%d", past, future)
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	db, wsID := seedTestDB(t)
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	ctx := gormrepo.WithWorkspaceID(context.Background(), wsID)

	count := func(model any) int64 {
		var n int64
		db.WithContext(ctx).Model(model).Count(&n)
		return n
	}
	before := [3]int64{count(&gormrepo.User{}), count(&gormrepo.Entry{}), count(&gormrepo.Plan{})}

	junk := gormrepo.User{WorkspaceID: wsID, Email: "visitor@example.com", Name: "Visitor", PasswordHash: "x", Active: true}
	if err := db.WithContext(ctx).Create(&junk).Error; err != nil {
		t.Fatalf("create junk user: %v", err)
	}
	// A visitor's admin action leaves an audit event, which the reset must clear
	// despite the append-only trigger.
	audit := gormrepo.AuditLog{
		WorkspaceID: wsID, At: now, ActorUserID: 1, ActorRole: "admin",
		Action: "project.create", TargetType: "project", TargetID: 1, Changes: "{}", CreatedAt: now,
	}
	if err := db.WithContext(ctx).Create(&audit).Error; err != nil {
		t.Fatalf("create audit event: %v", err)
	}
	if err := Seed(context.Background(), db, now); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	after := [3]int64{count(&gormrepo.User{}), count(&gormrepo.Entry{}), count(&gormrepo.Plan{})}
	if before != after {
		t.Errorf("counts changed across reset: before=%v after=%v", before, after)
	}
	var visitors, audits, triggers int64
	db.WithContext(ctx).Model(&gormrepo.User{}).Where("email = ?", "visitor@example.com").Count(&visitors)
	if visitors != 0 {
		t.Error("visitor-created user survived the reset")
	}
	db.WithContext(ctx).Model(&gormrepo.AuditLog{}).Count(&audits)
	if audits != 0 {
		t.Errorf("audit events survived the reset: %d", audits)
	}
	db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name = 'prevent_audit_log_delete'").Scan(&triggers)
	if triggers != 1 {
		t.Errorf("audit delete trigger count = %d, want 1", triggers)
	}
}

func TestSeedRefusesAnotherWorkspace(t *testing.T) {
	db, _ := seedTestDB(t)
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	other := gormrepo.Workspace{Slug: "real.example", CreatedAt: now}
	if err := db.WithContext(gormrepo.WithBypassTenancy(context.Background())).Create(&other).Error; err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	if err := Seed(context.Background(), db, now); err == nil {
		t.Fatal("seed accepted a database holding another workspace")
	}
}

func TestAccounts(t *testing.T) {
	got := Accounts()
	if len(got) != 3 {
		t.Fatalf("accounts = %d, want 3", len(got))
	}
	roles := map[string]bool{}
	for _, a := range got {
		roles[a.Role] = true
		if a.Password != "demo" {
			t.Errorf("%s has password %q, want demo", a.Email, a.Password)
		}
	}
	for _, role := range []string{"admin", "lead", "member"} {
		if !roles[role] {
			t.Errorf("missing %s account", role)
		}
	}
}
