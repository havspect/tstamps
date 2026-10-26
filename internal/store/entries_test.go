package store

import (
	"errors"
	"testing"
	"time"
)

// seedEntryWorld creates a team, a project and a user for the entry tests.
func seedEntryWorld(t *testing.T) (*Store, *User, *Project) {
	t.Helper()
	s := openTestStore(t)
	u := &User{Email: "ada@example.com", Name: "Ada", PasswordHash: "x", Active: true}
	if err := s.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	team := &Team{Name: "Platform"}
	if err := s.CreateTeam(team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	p := &Project{TeamID: team.ID, Name: "Timeline"}
	if err := s.CreateProject(p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return s, u, p
}

func at(hour int) time.Time {
	return time.Date(2026, 10, 19, hour, 0, 0, 0, time.UTC)
}

func finished(userID, projectID uint, from, to time.Time) *Entry {
	return &Entry{UserID: userID, ProjectID: projectID, StartAt: from, EndAt: &to}
}

func TestOneRunningEntryPerUser(t *testing.T) {
	s, u, p := seedEntryWorld(t)

	first := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9)}
	if err := s.CreateEntry(first); err != nil {
		t.Fatalf("first running entry: %v", err)
	}

	// The partial unique index is the authority, so the race loser sees this too.
	err := s.CreateEntry(&Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(10)})
	if !errors.Is(err, ErrEntryRunning) {
		t.Fatalf("second running entry = %v, want ErrEntryRunning", err)
	}

	// Finished entries are never limited.
	if err := s.CreateEntry(finished(u.ID, p.ID, at(6), at(7))); err != nil {
		t.Fatalf("finished entry alongside a running one: %v", err)
	}
	if err := s.CreateEntry(finished(u.ID, p.ID, at(7), at(8))); err != nil {
		t.Fatalf("second finished entry: %v", err)
	}
}

func TestSoftDeleteFreesTheRunningSlot(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	running := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9)}
	if err := s.CreateEntry(running); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.SoftDeleteEntry(running.ID, at(10)); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := s.CreateEntry(&Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(11)}); err != nil {
		t.Fatalf("create after deleting the running entry: %v", err)
	}

	// Deleting twice is not a silent success.
	if err := s.SoftDeleteEntry(running.ID, at(12)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestEntryRoundTripAndRestore(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	e := finished(u.ID, p.ID, at(9), at(11))
	e.Note = "drag, resize, snap"
	if err := s.CreateEntry(e); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.EntryByID(e.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Note != e.Note || !got.StartAt.Equal(e.StartAt) || got.EndAt == nil || !got.EndAt.Equal(*e.EndAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	e.Note = "edited"
	e.StartAt = at(8)
	if err := s.UpdateEntry(e.ID, at(9), map[string]any{"note": e.Note, "start_at": e.StartAt}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = s.EntryByID(e.ID)
	if err != nil {
		t.Fatalf("by id after update: %v", err)
	}
	if got.Note != "edited" || !got.StartAt.Equal(at(8)) {
		t.Fatalf("update not applied: %+v", got)
	}

	if err := s.SoftDeleteEntry(e.ID, at(12)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.RestoreEntry(e.ID, at(13)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err = s.EntryByID(e.ID)
	if err != nil {
		t.Fatalf("by id after restore: %v", err)
	}
	if got.DeletedAt != nil {
		t.Fatalf("restore left deleted_at set: %+v", got)
	}

	if err := s.RestoreEntry(999, at(13)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restore unknown = %v, want ErrNotFound", err)
	}
}

func TestStopRunning(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	running := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9), Note: "morning"}
	if err := s.CreateEntry(running); err != nil {
		t.Fatalf("create: %v", err)
	}

	// An empty note keeps the entry's own.
	if err := s.StopRunning(u.ID, at(11), ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	got, err := s.EntryByID(running.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.EndAt == nil || !got.EndAt.Equal(at(11)) || got.Note != "morning" {
		t.Fatalf("stop result: %+v", got)
	}

	if err := s.StopRunning(u.ID, at(12), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stop with nothing running = %v, want ErrNotFound", err)
	}

	// A note replaces the entry's own.
	second := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(13), Note: "old"}
	if err := s.CreateEntry(second); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if err := s.StopRunning(u.ID, at(14), "new"); err != nil {
		t.Fatalf("stop with note: %v", err)
	}
	got, err = s.EntryByID(second.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Note != "new" {
		t.Fatalf("note = %q, want new", got.Note)
	}

	if _, err := s.RunningEntry(u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RunningEntry after stop = %v, want ErrNotFound", err)
	}
}

func TestRestoreRunningEntryConflictsWithTheNewOne(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	first := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9)}
	if err := s.CreateEntry(first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := s.SoftDeleteEntry(first.ID, at(10)); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if err := s.CreateEntry(&Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(11)}); err != nil {
		t.Fatalf("create second: %v", err)
	}

	// Restoring the deleted running entry would be a second one, so the index
	// refuses it and the error is named, not a driver surprise.
	if err := s.RestoreEntry(first.ID, at(12)); !errors.Is(err, ErrEntryRunning) {
		t.Fatalf("restore = %v, want ErrEntryRunning", err)
	}
}

// A note-only update writes the note and nothing else, so a stop that landed
// after the caller read the entry is not undone by a stale end_at.
func TestUpdateEntryWritesOnlyTheNamedColumns(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	running := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9)}
	if err := s.CreateEntry(running); err != nil {
		t.Fatalf("create: %v", err)
	}
	stale, err := s.EntryByID(running.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}

	// The stop lands after the handler's read and before its write.
	if err := s.StopRunning(u.ID, at(10), ""); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := s.UpdateEntry(stale.ID, at(11), map[string]any{"note": "typed while stopping"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.EntryByID(stale.ID)
	if err != nil {
		t.Fatalf("by id after update: %v", err)
	}
	if got.EndAt == nil || !got.EndAt.Equal(at(10)) {
		t.Fatalf("end_at = %v, want the stop at %v to survive", got.EndAt, at(10))
	}
	if got.Note != "typed while stopping" {
		t.Errorf("note = %q, want the new note", got.Note)
	}
	if !got.UpdatedAt.Equal(at(11)) {
		t.Errorf("updated_at = %v, want %v", got.UpdatedAt, at(11))
	}
}

func TestSaveUserStopsTheRunningEntryOnDeactivation(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	if err := s.CreateEntry(&Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(9), Note: "keep me"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	u.Active = false
	if err := s.SaveUser(u, at(10)); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	got, err := s.UserByID(u.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Active {
		t.Fatal("user still active")
	}
	if _, err := s.RunningEntry(u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("running entry survived deactivation: %v", err)
	}
	var stopped Entry
	if err := s.db.Where("user_id = ?", u.ID).First(&stopped).Error; err != nil {
		t.Fatalf("load entry: %v", err)
	}
	if stopped.EndAt == nil || !stopped.EndAt.Equal(at(10)) || stopped.Note != DeactivationNote {
		t.Fatalf("stopped entry = %+v, want end %v and note %q", stopped, at(10), DeactivationNote)
	}
}

func TestMoveProjectRefusesAProjectWithEntries(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	other := &Team{Name: "Mobile"}
	if err := s.CreateTeam(other); err != nil {
		t.Fatalf("create team: %v", err)
	}

	if err := s.MoveProject(p.ID, other.ID, at(9)); err != nil {
		t.Fatalf("move an empty project: %v", err)
	}

	// Deleted rows count too, so a restored entry cannot land in another team.
	e := finished(u.ID, p.ID, at(9), at(11))
	if err := s.CreateEntry(e); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.SoftDeleteEntry(e.ID, at(12)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.MoveProject(p.ID, p.TeamID, at(13)); !errors.Is(err, ErrProjectHasEntries) {
		t.Fatalf("move a project with a deleted entry = %v, want ErrProjectHasEntries", err)
	}
}

func TestEntriesInRange(t *testing.T) {
	s, u, p := seedEntryWorld(t)

	if err := s.CreateEntry(finished(u.ID, p.ID, at(6), at(8))); err != nil {
		t.Fatalf("create before: %v", err)
	}
	inside := finished(u.ID, p.ID, at(10), at(12))
	if err := s.CreateEntry(inside); err != nil {
		t.Fatalf("create inside: %v", err)
	}
	deleted := finished(u.ID, p.ID, at(11), at(13))
	if err := s.CreateEntry(deleted); err != nil {
		t.Fatalf("create deleted: %v", err)
	}
	if err := s.SoftDeleteEntry(deleted.ID, at(14)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	running := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(15)}
	if err := s.CreateEntry(running); err != nil {
		t.Fatalf("create running: %v", err)
	}

	// from inclusive, to exclusive: [10:00, 16:00) keeps 10:00-12:00 and the running one.
	got, err := s.EntriesInRange(at(10), at(16), Scope{All: true})
	if err != nil {
		t.Fatalf("in range: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got[0].ID != inside.ID || got[1].ID != running.ID {
		t.Fatalf("wrong entries or order: %+v", got)
	}
	// The running entry starts after this range, so it is not in it.
	if got, err := s.EntriesInRange(at(10), at(13), Scope{All: true}); err != nil || len(got) != 1 {
		t.Fatalf("range before the running entry: %d entries, err=%v", len(got), err)
	}
}

func TestScopeNarrowsEntries(t *testing.T) {
	s := openTestStore(t)
	member := &User{Email: "linus@example.com", Name: "Linus", PasswordHash: "x"}
	lead := &User{Email: "grace@example.com", Name: "Grace", PasswordHash: "x"}
	other := &User{Email: "hedy@example.com", Name: "Hedy", PasswordHash: "x"}
	for _, u := range []*User{member, lead, other} {
		if err := s.CreateUser(u); err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	team := &Team{Name: "Platform"}
	if err := s.CreateTeam(team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	archivedTeam := &Team{Name: "Mobile"}
	if err := s.CreateTeam(archivedTeam); err != nil {
		t.Fatalf("create team: %v", err)
	}
	project := &Project{TeamID: team.ID, Name: "Timeline"}
	archivedProject := &Project{TeamID: archivedTeam.ID, Name: "Sync"}
	for _, p := range []*Project{project, archivedProject} {
		if err := s.CreateProject(p); err != nil {
			t.Fatalf("create project: %v", err)
		}
	}
	if err := s.CreateMembership(&TeamMembership{TeamID: team.ID, UserID: lead.ID, Role: RoleLead}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if err := s.CreateMembership(&TeamMembership{TeamID: archivedTeam.ID, UserID: other.ID, Role: RoleLead}); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if err := s.ArchiveTeam(archivedTeam.ID, at(1)); err != nil {
		t.Fatalf("archive team: %v", err)
	}
	archivedAt := at(1)
	archivedProject.ArchivedAt = &archivedAt
	if err := s.UpdateProject(archivedProject); err != nil {
		t.Fatalf("archive project: %v", err)
	}

	own := finished(member.ID, project.ID, at(9), at(10))
	led := finished(other.ID, project.ID, at(9), at(10))
	elsewhere := finished(other.ID, archivedProject.ID, at(9), at(10))
	for _, e := range []*Entry{own, led, elsewhere} {
		if err := s.CreateEntry(e); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}

	ledIDs, err := s.ProjectIDsLedBy(lead.ID)
	if err != nil {
		t.Fatalf("led projects: %v", err)
	}
	// The archived team grants nothing, but its archived project keeps its hours
	// when the team is still led.
	if len(ledIDs) != 1 || ledIDs[0] != project.ID {
		t.Fatalf("led projects = %v, want [%d]", ledIDs, project.ID)
	}
	if ids, err := s.ProjectIDsLedBy(other.ID); err != nil || len(ids) != 0 {
		t.Fatalf("archived team lead got projects %v, err=%v", ids, err)
	}

	cases := []struct {
		name  string
		scope Scope
		want  []uint
	}{
		{"member sees own", Scope{UserID: member.ID}, []uint{own.ID}},
		{"lead sees own and the team's", Scope{UserID: lead.ID, ProjectIDs: ledIDs}, []uint{own.ID, led.ID}},
		{"admin sees all", Scope{All: true}, []uint{own.ID, led.ID, elsewhere.ID}},
		{"zero scope sees nothing", Scope{}, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.EntriesInRange(at(8), at(11), tt.scope)
			if err != nil {
				t.Fatalf("in range: %v", err)
			}
			ids := make([]uint, 0, len(got))
			for i := range got {
				ids = append(ids, got[i].ID)
			}
			if len(ids) != len(tt.want) {
				t.Fatalf("ids = %v, want %v", ids, tt.want)
			}
			for i := range ids {
				if ids[i] != tt.want[i] {
					t.Fatalf("ids = %v, want %v", ids, tt.want)
				}
			}
		})
	}
}

func TestByIDsHelpers(t *testing.T) {
	s, u, p := seedEntryWorld(t)
	team, err := s.TeamByID(p.TeamID)
	if err != nil {
		t.Fatalf("team: %v", err)
	}

	users, err := s.UsersByIDs([]uint{u.ID, 999})
	if err != nil || len(users) != 1 || users[0].ID != u.ID {
		t.Fatalf("UsersByIDs = %v, err=%v", users, err)
	}
	projects, err := s.ProjectsByIDs([]uint{p.ID, 999})
	if err != nil || len(projects) != 1 || projects[0].ID != p.ID {
		t.Fatalf("ProjectsByIDs = %v, err=%v", projects, err)
	}
	teams, err := s.TeamsByIDs([]uint{team.ID, 999})
	if err != nil || len(teams) != 1 || teams[0].ID != team.ID {
		t.Fatalf("TeamsByIDs = %v, err=%v", teams, err)
	}
	for _, empty := range [][]uint{nil, {}} {
		if got, err := s.UsersByIDs(empty); err != nil || len(got) != 0 {
			t.Fatalf("UsersByIDs(%v) = %v, err=%v", empty, got, err)
		}
	}
}

// An entry from before the timezone column gets its owner's zone when the
// column is added; a set zone is never overwritten.
func TestMigrateAddsAndBackfillsEntryTimezone(t *testing.T) {
	s, _, p := seedEntryWorld(t)
	u := &User{Email: "berlin@example.com", Name: "Berlin", PasswordHash: "x", Timezone: "Europe/Berlin", Active: true}
	if err := s.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Rebuild the pre-timezone table so AutoMigrate has to add the column.
	if err := s.db.Exec(`DROP TABLE entries`).Error; err != nil {
		t.Fatalf("drop entries: %v", err)
	}
	if err := s.db.Exec(`CREATE TABLE entries (
		id integer PRIMARY KEY AUTOINCREMENT, workspace_id integer NOT NULL,
		user_id integer NOT NULL,
		project_id integer NOT NULL, start_at datetime NOT NULL, end_at datetime,
		note text, created_at datetime, updated_at datetime, deleted_at datetime)`).Error; err != nil {
		t.Fatalf("create old entries: %v", err)
	}
	if err := s.db.Exec(`INSERT INTO entries (workspace_id, user_id, project_id, start_at, end_at) VALUES (?, ?, ?, ?, ?)`,
		s.workspaceID, u.ID, p.ID, at(9), at(10)).Error; err != nil {
		t.Fatalf("insert old entry: %v", err)
	}
	// Pretend this is a pre-timezone database so Migrate takes the full path.
	if err := s.db.Exec(`PRAGMA user_version = 0`).Error; err != nil {
		t.Fatalf("reset schema version: %v", err)
	}

	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rows, err := s.EntriesInRange(at(0), at(23), Scope{UserID: u.ID})
	if err != nil || len(rows) != 1 {
		t.Fatalf("entries = %v, err=%v", rows, err)
	}
	if rows[0].Timezone != "Europe/Berlin" {
		t.Fatalf("backfilled timezone = %q, want Europe/Berlin", rows[0].Timezone)
	}

	kept := &Entry{UserID: u.ID, ProjectID: p.ID, StartAt: at(11), EndAt: ptrAt(at(12)), Timezone: "Australia/Sydney"}
	if err := s.CreateEntry(kept); err != nil {
		t.Fatalf("create kept entry: %v", err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	got, err := s.EntryByID(kept.ID)
	if err != nil || got.Timezone != "Australia/Sydney" {
		t.Fatalf("kept timezone = %q, err=%v; want Australia/Sydney", got.Timezone, err)
	}
}

func ptrAt(t time.Time) *time.Time { return &t }
