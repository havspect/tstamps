package entry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
)

var testNow = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

type mockClock struct {
	now time.Time
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

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

type fakeProjects struct {
	loggable map[uint]bool
}

func (p *fakeProjects) IsProjectLoggable(ctx context.Context, projectID uint) (bool, error) {
	return p.loggable[projectID], nil
}

type fakeRepo struct {
	entries map[uint]*Entry
	nextID  uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		entries: make(map[uint]*Entry),
		nextID:  1,
	}
}

func (r *fakeRepo) GetByID(ctx context.Context, id uint) (*Entry, error) {
	e, ok := r.entries[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *fakeRepo) GetRunning(ctx context.Context, userID uint) (*Entry, error) {
	for _, e := range r.entries {
		if e.UserID == userID && e.EndAt == nil && e.DeletedAt == nil {
			cp := *e
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) List(ctx context.Context, filter ListFilter) ([]Entry, error) {
	var out []Entry
	for _, e := range r.entries {
		if !filter.IncludeDeleted && e.DeletedAt != nil {
			continue
		}
		if filter.UserID != 0 && e.UserID != filter.UserID {
			continue
		}
		if filter.ProjectID != 0 && e.ProjectID != filter.ProjectID {
			continue
		}
		if filter.From != nil && e.EndAt != nil && !e.EndAt.After(*filter.From) {
			continue
		}
		if filter.To != nil && !e.StartAt.Before(*filter.To) {
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

func (r *fakeRepo) Create(ctx context.Context, e *Entry) error {
	if e.EndAt == nil {
		for _, other := range r.entries {
			if other.UserID == e.UserID && other.EndAt == nil && other.DeletedAt == nil {
				return ErrRunningExists
			}
		}
	}
	e.ID = r.nextID
	r.nextID++
	cp := *e
	r.entries[e.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(ctx context.Context, e *Entry) error {
	if _, ok := r.entries[e.ID]; !ok {
		return ErrNotFound
	}
	cp := *e
	r.entries[e.ID] = &cp
	return nil
}

func (r *fakeRepo) SoftDelete(ctx context.Context, id uint, deletedAt time.Time) error {
	e, ok := r.entries[id]
	if !ok || e.DeletedAt != nil {
		return ErrNotFound
	}
	e.DeletedAt = &deletedAt
	return nil
}

func (r *fakeRepo) Restore(ctx context.Context, id uint) error {
	e, ok := r.entries[id]
	if !ok {
		return ErrNotFound
	}
	e.DeletedAt = nil
	return nil
}

func testContext(user *auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), user)
}

func newTestService() (*Service, *fakeRepo, *fakeAudit, *fakeProjects, *mockClock) {
	repo := newFakeRepo()
	audit := &fakeAudit{}
	projects := &fakeProjects{
		loggable: map[uint]bool{1: true, 2: false},
	}
	clk := &mockClock{now: testNow}
	svc := NewService(
		repo,
		&fakeUOW{},
		audit,
		projects,
		authz.NewPolicyEngine(),
		clk,
	)
	return svc, repo, audit, projects, clk
}

func TestLogEntry(t *testing.T) {
	svc, repo, _, _, _ := newTestService()
	user := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	ctx := testContext(user)

	// 1. Unloggable project
	_, err := svc.LogEntry(ctx, 2, testNow.Add(-time.Hour), testNow, "UTC", "unloggable")
	if !errors.Is(err, ErrProjectNotLoggable) {
		t.Fatalf("LogEntry unloggable project: got %v, want ErrProjectNotLoggable", err)
	}

	// 2. Invalid range (end <= start)
	_, err = svc.LogEntry(ctx, 1, testNow, testNow.Add(-time.Hour), "UTC", "invalid")
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("LogEntry end < start: got %v, want ErrInvalidRange", err)
	}

	// 3. Too long (> 24h)
	_, err = svc.LogEntry(ctx, 1, testNow.Add(-25*time.Hour), testNow, "UTC", "too long")
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("LogEntry too long: got %v, want ErrTooLong", err)
	}

	// 4. Valid entry
	entry, err := svc.LogEntry(ctx, 1, testNow.Add(-time.Hour), testNow, "Europe/Berlin", "valid work")
	if err != nil {
		t.Fatalf("LogEntry valid: unexpected error %v", err)
	}
	if entry.ID == 0 || entry.UserID != 10 || entry.ProjectID != 1 {
		t.Errorf("entry fields mismatch: %+v", entry)
	}
	if len(repo.entries) != 1 {
		t.Errorf("expected 1 entry in repo, got %d", len(repo.entries))
	}

	// 5. Read-only token scope rejects mutation
	roUser := &auth.Principal{WorkspaceID: 1, UserID: 10, Scopes: []string{"tstamps.read"}}
	roCtx := testContext(roUser)
	_, err = svc.LogEntry(roCtx, 1, testNow.Add(-time.Hour), testNow, "UTC", "")
	if !errors.Is(err, authz.ErrReadOnly) {
		t.Fatalf("LogEntry with read-only token: got %v, want ErrReadOnly", err)
	}
}

func TestPatchEntry(t *testing.T) {
	svc, repo, _, _, _ := newTestService()
	user1 := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	user2 := &auth.Principal{WorkspaceID: 1, UserID: 20, Roles: []string{"member"}}

	end := testNow.Add(-10 * time.Minute)
	e := &Entry{
		WorkspaceID: 1,
		UserID:      10,
		ProjectID:   1,
		StartAt:     testNow.Add(-time.Hour),
		EndAt:       &end,
		Note:        "initial",
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf("create fixture: %v", err)
	}

	// 1. Foreign user cannot modify entry
	note := "hacked"
	_, err := svc.PatchEntry(testContext(user2), e.ID, nil, nil, nil, nil, &note)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("PatchEntry foreign user: got %v, want ErrForbidden", err)
	}

	// 2. Owner modifies note
	newNote := "updated"
	patched, err := svc.PatchEntry(testContext(user1), e.ID, nil, nil, nil, nil, &newNote)
	if err != nil {
		t.Fatalf("PatchEntry note: %v", err)
	}
	if patched.Note != "updated" {
		t.Errorf("expected note %q, got %q", "updated", patched.Note)
	}

	// 3. Move to unloggable project rejected
	unloggable := uint(2)
	_, err = svc.PatchEntry(testContext(user1), e.ID, &unloggable, nil, nil, nil, nil)
	if !errors.Is(err, ErrProjectNotLoggable) {
		t.Fatalf("PatchEntry to unloggable project: got %v, want ErrProjectNotLoggable", err)
	}

	// 4. Retiming running entry without end rejected
	running := &Entry{
		WorkspaceID: 1,
		UserID:      10,
		ProjectID:   1,
		StartAt:     testNow.Add(-time.Hour),
		EndAt:       nil,
	}
	if err := repo.Create(context.Background(), running); err != nil {
		t.Fatalf("create running fixture: %v", err)
	}
	newStart := testNow.Add(-2 * time.Hour)
	_, err = svc.PatchEntry(testContext(user1), running.ID, nil, &newStart, nil, nil, nil)
	if !errors.Is(err, ErrRetimeRunning) {
		t.Fatalf("PatchEntry retime running: got %v, want ErrRetimeRunning", err)
	}
}

func TestDeleteAndRestoreEntry(t *testing.T) {
	svc, repo, audit, _, _ := newTestService()
	member := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	otherMember := &auth.Principal{WorkspaceID: 1, UserID: 20, Roles: []string{"member"}}
	admin := &auth.Principal{WorkspaceID: 1, UserID: 99, Roles: []string{"admin"}}

	end := testNow.Add(-time.Minute)
	e := &Entry{
		WorkspaceID: 1,
		UserID:      10,
		ProjectID:   1,
		StartAt:     testNow.Add(-time.Hour),
		EndAt:       &end,
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf("create fixture: %v", err)
	}

	// 1. Other user cannot delete
	err := svc.DeleteEntry(testContext(otherMember), e.ID)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Delete foreign: got %v, want ErrForbidden", err)
	}

	// 2. Owner can delete
	err = svc.DeleteEntry(testContext(member), e.ID)
	if err != nil {
		t.Fatalf("Delete own: %v", err)
	}
	stored, _ := repo.GetByID(context.Background(), e.ID)
	if stored.DeletedAt == nil {
		t.Fatal("entry was not soft deleted")
	}

	// 3. Non-admin cannot restore
	_, err = svc.RestoreEntry(testContext(member), e.ID)
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Restore non-admin: got %v, want ErrForbidden", err)
	}

	// 4. Admin restores successfully and records audit
	restored, err := svc.RestoreEntry(testContext(admin), e.ID)
	if err != nil {
		t.Fatalf("Restore admin: %v", err)
	}
	if restored.DeletedAt != nil {
		t.Fatal("restored entry still has DeletedAt")
	}
	if len(audit.records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(audit.records))
	}
	if audit.records[0].action != "entry.restore" || audit.records[0].targetID != e.ID {
		t.Errorf("audit record mismatch: %+v", audit.records[0])
	}
}

func TestTimerOperations(t *testing.T) {
	svc, _, _, _, clk := newTestService()
	member := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	ctx := testContext(member)

	// 1. GetRunning returns nil initially
	running, err := svc.GetRunning(ctx)
	if err != nil || running != nil {
		t.Fatalf("GetRunning initial: got (%+v, %v), want (nil, nil)", running, err)
	}

	// 2. StartTimer
	started, err := svc.StartTimer(ctx, 1, "UTC", "first timer")
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if started.ID == 0 || started.EndAt != nil {
		t.Fatalf("StartTimer returned invalid entry: %+v", started)
	}

	// Advance clock by 1 hour
	clk.Advance(time.Hour)

	// 3. Second StartTimer conflicts
	_, err = svc.StartTimer(ctx, 1, "UTC", "second timer")
	var conflict *AlreadyRunning
	if !errors.As(err, &conflict) {
		t.Fatalf("StartTimer duplicate: got %v, want AlreadyRunning", err)
	}
	if conflict.Entry.ID != started.ID {
		t.Errorf("conflict entry ID = %d, want %d", conflict.Entry.ID, started.ID)
	}

	// 4. StopTimer keeps existing note if blank passed
	blank := ""
	stopped, err := svc.StopTimer(ctx, &blank)
	if err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if stopped.EndAt == nil || !stopped.EndAt.Equal(testNow.Add(time.Hour)) {
		t.Errorf("stopped EndAt mismatch: %v", stopped.EndAt)
	}
	if stopped.Note != "first timer" {
		t.Errorf("expected note preserved 'first timer', got %q", stopped.Note)
	}

	// 5. StopTimer when none running returns ErrNotFound
	_, err = svc.StopTimer(ctx, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("StopTimer with none running: got %v, want ErrNotFound", err)
	}
}

func TestStopTimerExceeding24HoursLeavesRunning(t *testing.T) {
	svc, repo, _, _, _ := newTestService()
	member := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	ctx := testContext(member)

	e := &Entry{
		WorkspaceID: 1,
		UserID:      10,
		ProjectID:   1,
		StartAt:     testNow.Add(-25 * time.Hour),
		EndAt:       nil,
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf("create fixture: %v", err)
	}

	_, err := svc.StopTimer(ctx, nil)
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("StopTimer too long: got %v, want ErrTooLong", err)
	}

	running, err := svc.GetRunning(ctx)
	if err != nil || running == nil {
		t.Fatalf("expected timer to stay running, got %+v, %v", running, err)
	}
}

func TestListEntries(t *testing.T) {
	svc, repo, _, _, _ := newTestService()
	member := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	admin := &auth.Principal{WorkspaceID: 1, UserID: 99, Roles: []string{"admin"}}

	end := testNow.Add(-time.Hour)
	del := testNow.Add(-30 * time.Minute)
	repo.Create(context.Background(), &Entry{WorkspaceID: 1, UserID: 10, ProjectID: 1, StartAt: testNow.Add(-2 * time.Hour), EndAt: &end})
	repo.Create(context.Background(), &Entry{WorkspaceID: 1, UserID: 20, ProjectID: 1, StartAt: testNow.Add(-2 * time.Hour), EndAt: &end})
	repo.Create(context.Background(), &Entry{WorkspaceID: 1, UserID: 10, ProjectID: 1, StartAt: testNow.Add(-3 * time.Hour), EndAt: &end, DeletedAt: &del})

	// Member only sees own non-deleted entries
	entries, err := svc.ListEntries(testContext(member), ListFilter{})
	if err != nil {
		t.Fatalf("ListEntries member: %v", err)
	}
	if len(entries) != 1 || entries[0].UserID != 10 {
		t.Errorf("expected 1 entry for user 10, got %d", len(entries))
	}

	// Member cannot request IncludeDeleted
	_, err = svc.ListEntries(testContext(member), ListFilter{IncludeDeleted: true})
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("member IncludeDeleted: got %v, want ErrForbidden", err)
	}

	// Admin can request IncludeDeleted and see all
	adminEntries, err := svc.ListEntries(testContext(admin), ListFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListEntries admin: %v", err)
	}
	if len(adminEntries) != 3 {
		t.Errorf("expected 3 entries for admin, got %d", len(adminEntries))
	}
}

func TestOverlapRules(t *testing.T) {
	svc, repo, _, projects, clk := newTestService()
	projects.loggable[2] = true
	user := &auth.Principal{WorkspaceID: 1, UserID: 10, Roles: []string{"member"}}
	other := &auth.Principal{WorkspaceID: 1, UserID: 20, Roles: []string{"member"}}
	admin := &auth.Principal{WorkspaceID: 1, UserID: 99, Roles: []string{"admin"}}
	ctx := testContext(user)

	start := testNow.Add(-2 * time.Hour)
	end := start.Add(time.Hour)
	if _, err := svc.LogEntry(ctx, 1, start, end, "UTC", "first"); err != nil {
		t.Fatalf("log first: %v", err)
	}

	// Partial overlap on the same project is rejected.
	if _, err := svc.LogEntry(ctx, 1, start.Add(30*time.Minute), end.Add(30*time.Minute), "UTC", "clash"); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlapping entry: got %v, want ErrOverlap", err)
	}

	// Adjacent entries are fine, as is the same time on another project or by another user.
	next, err := svc.LogEntry(ctx, 1, end, end.Add(time.Hour), "UTC", "next")
	if err != nil {
		t.Fatalf("adjacent entry: %v", err)
	}
	if _, err := svc.LogEntry(ctx, 2, start, end, "UTC", "parallel"); err != nil {
		t.Fatalf("same time on another project: %v", err)
	}
	if _, err := svc.LogEntry(testContext(other), 1, start, end, "UTC", "colleague"); err != nil {
		t.Fatalf("same time by another user: %v", err)
	}

	// Moving an entry onto another one on the same project is rejected.
	movedStart, movedEnd := start.Add(-time.Hour), start.Add(time.Hour)
	if _, err := svc.PatchEntry(ctx, next.ID, nil, &movedStart, &movedEnd, nil, nil); !errors.Is(err, ErrOverlap) {
		t.Fatalf("retime onto overlap: got %v, want ErrOverlap", err)
	}

	// A restored entry may not overlap a live one either.
	if err := svc.DeleteEntry(ctx, next.ID); err != nil {
		t.Fatalf("delete next: %v", err)
	}
	if _, err := svc.LogEntry(ctx, 1, end.Add(15*time.Minute), end.Add(45*time.Minute), "UTC", "fills the gap"); err != nil {
		t.Fatalf("log after delete: %v", err)
	}
	if _, err := svc.RestoreEntry(testContext(admin), next.ID); !errors.Is(err, ErrOverlap) {
		t.Fatalf("restore onto overlap: got %v, want ErrOverlap", err)
	}

	// A running timer may not cover an existing entry on the same project.
	futureStart := testNow.Add(4 * time.Minute)
	futureEnd := testNow.Add(5 * time.Minute)
	if _, err := svc.LogEntry(ctx, 1, futureStart, futureEnd, "UTC", "about to start"); err != nil {
		t.Fatalf("log future entry: %v", err)
	}
	if _, err := svc.StartTimer(ctx, 1, "UTC", "timer"); !errors.Is(err, ErrOverlap) {
		t.Fatalf("start timer over entry: got %v, want ErrOverlap", err)
	}

	// Stopping into an entry on the same project is rejected and leaves the timer running.
	clk.Advance(10 * time.Minute)
	timer, err := svc.StartTimer(ctx, 1, "UTC", "timer")
	if err != nil {
		t.Fatalf("start timer: %v", err)
	}
	conflictStart := clk.Now().Add(5 * time.Minute)
	conflictEnd := conflictStart.Add(15 * time.Minute)
	if err := repo.Create(context.Background(), &Entry{
		WorkspaceID: 1, UserID: 10, ProjectID: 1, StartAt: conflictStart, EndAt: &conflictEnd,
	}); err != nil {
		t.Fatalf("create conflicting entry: %v", err)
	}
	clk.Advance(15 * time.Minute)
	if _, err := svc.StopTimer(ctx, nil); !errors.Is(err, ErrOverlap) {
		t.Fatalf("stop timer onto overlap: got %v, want ErrOverlap", err)
	}
	running, err := svc.GetRunning(ctx)
	if err != nil || running == nil || running.ID != timer.ID {
		t.Fatalf("timer should stay running: got %+v, %v", running, err)
	}
}
