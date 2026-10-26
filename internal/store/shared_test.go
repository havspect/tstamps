package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestRegistryScopesEveryWorkspace is the core promise of the shared file: two
// workspaces live in one database, and no read in one can see the other's rows.
func TestRegistryScopesEveryWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tstamps.db")
	reg, err := OpenRegistry(path)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	if err := reg.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	alpha, beta := &Workspace{Slug: "alpha"}, &Workspace{Slug: "beta"}
	for _, w := range []*Workspace{alpha, beta} {
		if err := reg.CreateWorkspace(w); err != nil {
			t.Fatalf("create %s: %v", w.Slug, err)
		}
	}
	as, bs := reg.ForWorkspace(alpha.ID), reg.ForWorkspace(beta.ID)

	// The same email is two accounts, one per workspace.
	au, bu := &User{Email: "ada@example.com", Name: "Ada Alpha", PasswordHash: "x"},
		&User{Email: "ada@example.com", Name: "Ada Beta", PasswordHash: "y"}
	for _, u := range []struct {
		st *Store
		u  *User
	}{{as, au}, {bs, bu}} {
		if err := u.st.CreateUser(u.u); err != nil {
			t.Fatalf("create user in %s: %v", u.u.Name, err)
		}
	}
	got, err := as.UserByEmail("ada@example.com")
	if err != nil || got.Name != "Ada Alpha" {
		t.Fatalf("alpha email = %+v, %v; want Ada Alpha", got, err)
	}
	if _, err := as.UserByID(bu.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta user: %v, want ErrNotFound", err)
	}
	if err := as.SetUserPassword(bu.ID, "new", false, 0, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha reset beta password: %v, want ErrNotFound", err)
	}
	if err := as.SaveUser(&User{ID: bu.ID, Name: "hijack"}, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha save beta user: %v, want ErrNotFound", err)
	}

	// Teams, projects and entries are scoped the same way.
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	at, bt := &Team{Name: "Platform"}, &Team{Name: "Mobile"}
	_ = as.CreateTeam(at)
	_ = bs.CreateTeam(bt)
	ap, bp := &Project{TeamID: at.ID, Name: "Timeline"}, &Project{TeamID: bt.ID, Name: "Sync"}
	if err := as.CreateProject(ap); err != nil {
		t.Fatalf("create alpha project: %v", err)
	}
	if err := bs.CreateProject(bp); err != nil {
		t.Fatalf("create beta project: %v", err)
	}
	if _, err := as.ProjectByID(bp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta project: %v, want ErrNotFound", err)
	}
	if _, err := as.TeamByID(bt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta team: %v, want ErrNotFound", err)
	}
	if err := as.ArchiveTeam(bt.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha archive beta team: %v, want ErrNotFound", err)
	}
	if err := as.ReplaceMemberships(bt.ID, nil, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha edit beta memberships: %v, want ErrNotFound", err)
	}

	end := now.Add(time.Hour)
	for _, e := range []struct {
		st *Store
		e  *Entry
	}{
		{as, &Entry{UserID: au.ID, ProjectID: ap.ID, StartAt: now, EndAt: &end, Timezone: "UTC"}},
		{bs, &Entry{UserID: bu.ID, ProjectID: bp.ID, StartAt: now, EndAt: &end, Timezone: "UTC"}},
	} {
		if err := e.st.CreateEntry(e.e); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}
	from, to := now.Add(-time.Hour), now.Add(2*time.Hour)
	entries, err := as.EntriesInRange(from, to, Scope{All: true})
	if err != nil {
		t.Fatalf("alpha entries: %v", err)
	}
	if len(entries) != 1 || entries[0].UserID != au.ID {
		t.Fatalf("alpha entries = %+v, want only its own", entries)
	}

	// A running entry per workspace is legal, because the partial index is per
	// workspace, and an entry ID from beta is invisible in alpha.
	alphaRunning := &Entry{UserID: au.ID, ProjectID: ap.ID, StartAt: now, Timezone: "UTC"}
	betaRunning := &Entry{UserID: bu.ID, ProjectID: bp.ID, StartAt: now, Timezone: "UTC"}
	if err := as.CreateEntry(alphaRunning); err != nil {
		t.Fatalf("create alpha running entry: %v", err)
	}
	if err := bs.CreateEntry(betaRunning); err != nil {
		t.Fatalf("create beta running entry: %v", err)
	}
	if _, err := as.EntryByID(betaRunning.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha read beta entry: %v, want ErrNotFound", err)
	}
	if _, err := as.RunningEntry(au.ID); err != nil {
		t.Fatalf("alpha running entry: %v", err)
	}

	// Sessions, tokens and audit events are workspace-local too.
	if err := as.CreateSession(&Session{UserID: au.ID, TokenHash: "same-hash", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create alpha session: %v", err)
	}
	if _, err := bs.SessionByTokenHash("same-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("beta session lookup: %v, want ErrNotFound", err)
	}
	betaSession := &Session{UserID: bu.ID, TokenHash: "beta-session", ExpiresAt: now.Add(time.Hour)}
	if err := bs.CreateSession(betaSession); err != nil {
		t.Fatalf("create beta session: %v", err)
	}
	if err := as.DeleteSession(betaSession.ID); err != nil {
		t.Fatalf("alpha delete beta session: %v", err)
	}
	if _, err := bs.SessionByTokenHash("beta-session"); err != nil {
		t.Fatalf("beta session was deleted from alpha: %v", err)
	}
	if err := as.CreateAPIToken(&APIToken{UserID: au.ID, Name: "cli", TokenHash: "same-token"}); err != nil {
		t.Fatalf("create alpha token: %v", err)
	}
	if _, err := bs.APITokenByTokenHash("same-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("beta token lookup: %v, want ErrNotFound", err)
	}
	betaToken := &APIToken{UserID: bu.ID, Name: "cli", TokenHash: "beta-token"}
	if err := bs.CreateAPIToken(betaToken); err != nil {
		t.Fatalf("create beta token: %v", err)
	}
	if err := as.RevokeAPIToken(betaToken.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alpha revoke beta token: %v, want ErrNotFound", err)
	}
	if tok, err := bs.APITokenByTokenHash("beta-token"); err != nil || tok.RevokedAt != nil {
		t.Fatalf("beta token = %+v, %v; want still live", tok, err)
	}
	if err := as.RecordAudit(now, AuditActor{UserID: au.ID, Role: "admin"}, "project.create", "project", ap.ID, Change{}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	events, err := bs.AuditEvents(AuditFilter{})
	if err != nil || len(events) != 0 {
		t.Fatalf("beta audit = %v, %v; want none", events, err)
	}

	// One file holds both workspaces: this is the architecture, not two stores
	// that happen to point at a copy.
	all, err := reg.Workspaces()
	if err != nil || len(all) != 2 {
		t.Fatalf("workspaces = %v, %v; want two", all, err)
	}
}
