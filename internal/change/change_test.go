package change

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

type testClock struct {
	now time.Time
}

func (c testClock) Now() time.Time { return c.now }

func setupStore(t *testing.T) (*store.Store, clock.Clock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clk := testClock{now: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)}
	return st, clk
}

func TestUsersChangeCascade(t *testing.T) {
	st, clk := setupStore(t)
	mod := New(st, clk)
	adminActor := Actor{UserID: 1, Role: "admin"}

	// Create admin and target user
	admin := &store.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true, Active: true, PasswordHash: "h1"}
	if _, err := mod.Users.Create(context.Background(), adminActor, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	user := &store.User{Email: "user@example.com", Name: "User", IsAdmin: false, Active: true, PasswordHash: "h2"}
	if _, err := mod.Users.Create(context.Background(), adminActor, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create team and project, and start a running timer for user
	team, err := mod.Teams.Create(context.Background(), adminActor, &store.Team{Name: "Backend"})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	project, err := mod.Projects.Create(context.Background(), adminActor, team.ID, "API", "#000")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	start := clk.Now().Add(-time.Hour)
	running := &store.Entry{UserID: user.ID, ProjectID: project.ID, StartAt: start, Timezone: "UTC"}
	if err := st.CreateEntry(running); err != nil {
		t.Fatalf("create running entry: %v", err)
	}

	// Deactivate user via change.Users.Update
	inactive := false
	updated, err := mod.Users.Update(context.Background(), adminActor, user.ID, UserPatch{Active: &inactive})
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if updated.Active {
		t.Fatal("expected user to be inactive")
	}

	// Verify running entry was stopped by cascade
	stopped, err := st.EntryByID(running.ID)
	if err != nil {
		t.Fatalf("entry by id: %v", err)
	}
	if stopped.EndAt == nil {
		t.Fatal("expected running entry to have end_at set")
	}

	// Verify audit log has user.create, user.update and entry.stop
	events, err := st.AuditEvents(store.AuditFilter{ActorUserID: adminActor.UserID})
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	actions := make(map[string]bool)
	for _, ev := range events {
		actions[ev.Action] = true
	}
	if !actions["user.update"] || !actions["entry.stop"] {
		t.Fatalf("missing audit actions: %+v", actions)
	}
}
