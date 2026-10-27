package team_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/team"
)

type mockClock struct {
	now time.Time
}

func (m mockClock) Now() time.Time {
	return m.now
}

type mockRepo struct {
	teams       map[uint]*team.Team
	memberships map[uint][]team.Membership
	nextID      uint
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		teams:       make(map[uint]*team.Team),
		memberships: make(map[uint][]team.Membership),
		nextID:      1,
	}
}

func (m *mockRepo) GetByID(ctx context.Context, id uint) (*team.Team, error) {
	t, ok := m.teams[id]
	if !ok {
		return nil, team.ErrNotFound
	}
	cpy := *t
	return &cpy, nil
}

func (m *mockRepo) List(ctx context.Context, includeArchived bool) ([]team.TeamWithMembers, error) {
	var out []team.TeamWithMembers
	for _, t := range m.teams {
		if !includeArchived && t.ArchivedAt != nil {
			continue
		}
		out = append(out, team.TeamWithMembers{
			Team:    *t,
			Members: m.memberships[t.ID],
		})
	}
	return out, nil
}

func (m *mockRepo) Create(ctx context.Context, t *team.Team) error {
	t.ID = m.nextID
	m.nextID++
	cpy := *t
	m.teams[t.ID] = &cpy
	return nil
}

func (m *mockRepo) Update(ctx context.Context, t *team.Team) error {
	if _, ok := m.teams[t.ID]; !ok {
		return team.ErrNotFound
	}
	cpy := *t
	m.teams[t.ID] = &cpy
	return nil
}

func (m *mockRepo) Archive(ctx context.Context, id uint, at time.Time) error {
	t, ok := m.teams[id]
	if !ok {
		return team.ErrNotFound
	}
	if t.ArchivedAt == nil {
		t.ArchivedAt = &at
	}
	return nil
}

func (m *mockRepo) ReplaceMemberships(ctx context.Context, teamID uint, members []team.Membership) error {
	if _, ok := m.teams[teamID]; !ok {
		return team.ErrNotFound
	}
	m.memberships[teamID] = members
	return nil
}

type mockArchiver struct {
	archivedTeams []uint
}

func (a *mockArchiver) ArchiveProjectsByTeam(ctx context.Context, teamID uint, at time.Time) error {
	a.archivedTeams = append(a.archivedTeams, teamID)
	return nil
}

type mockAuditor struct {
	events []string
}

func (a *mockAuditor) Record(ctx context.Context, action, targetType string, targetID uint, changes any) error {
	a.events = append(a.events, action)
	return nil
}

func adminCtx() context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1,
		UserID:      1,
		Roles:       []string{"admin"},
	})
}

func leadCtx(ledTeamID uint) context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1,
		UserID:      2,
		Roles:       []string{"lead"},
		LedTeamIDs:  []uint{ledTeamID},
	})
}

func memberCtx() context.Context {
	return auth.WithPrincipal(context.Background(), &auth.Principal{
		WorkspaceID: 1,
		UserID:      3,
		Roles:       []string{"member"},
	})
}

func TestCreateTeam(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := team.New(repo, nil, auditor, mockClock{now: now})

	t.Run("admin authorization and validation", func(t *testing.T) {
		// Unauthorized without principal
		if _, err := svc.CreateTeam(context.Background(), "Engineering"); !errors.Is(err, authz.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}

		// Forbidden with member principal
		if _, err := svc.CreateTeam(memberCtx(), "Engineering"); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}

		// Empty name
		if _, err := svc.CreateTeam(adminCtx(), "   "); !errors.Is(err, team.ErrInvalidName) {
			t.Errorf("expected ErrInvalidName, got %v", err)
		}
	})

	t.Run("success creates team and records audit", func(t *testing.T) {
		res, err := svc.CreateTeam(adminCtx(), "Platform")
		if err != nil {
			t.Fatalf("CreateTeam failed: %v", err)
		}
		if res.ID == 0 || res.Name != "Platform" {
			t.Errorf("unexpected team: %+v", res)
		}
		if len(auditor.events) == 0 || auditor.events[len(auditor.events)-1] != "team.create" {
			t.Errorf("expected audit event team.create, got %v", auditor.events)
		}
	})
}

func TestRenameTeam(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := team.New(repo, nil, auditor, mockClock{now: now})

	created, _ := svc.CreateTeam(adminCtx(), "Core")

	t.Run("validation and not found", func(t *testing.T) {
		if _, err := svc.RenameTeam(adminCtx(), created.ID, ""); !errors.Is(err, team.ErrInvalidName) {
			t.Errorf("expected ErrInvalidName, got %v", err)
		}
		if _, err := svc.RenameTeam(adminCtx(), 999, "Renamed"); !errors.Is(err, team.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("success renames and audits", func(t *testing.T) {
		renamed, err := svc.RenameTeam(adminCtx(), created.ID, "Core Engine")
		if err != nil {
			t.Fatalf("RenameTeam failed: %v", err)
		}
		if renamed.Name != "Core Engine" {
			t.Errorf("expected name 'Core Engine', got %s", renamed.Name)
		}
		if auditor.events[len(auditor.events)-1] != "team.rename" {
			t.Errorf("expected team.rename audit, got %v", auditor.events)
		}
	})
}

func TestArchiveTeam(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	archiver := &mockArchiver{}
	auditor := &mockAuditor{}
	svc := team.New(repo, archiver, auditor, mockClock{now: now})

	created, _ := svc.CreateTeam(adminCtx(), "Operations")

	t.Run("success cascades project archiving", func(t *testing.T) {
		err := svc.ArchiveTeam(adminCtx(), created.ID)
		if err != nil {
			t.Fatalf("ArchiveTeam failed: %v", err)
		}
		if len(archiver.archivedTeams) != 1 || archiver.archivedTeams[0] != created.ID {
			t.Errorf("expected cascade archive for team %d, got %v", created.ID, archiver.archivedTeams)
		}
		if auditor.events[len(auditor.events)-1] != "team.archive" {
			t.Errorf("expected team.archive audit, got %v", auditor.events)
		}
	})

	t.Run("idempotent repeat archive does nothing", func(t *testing.T) {
		archiver.archivedTeams = nil
		eventCount := len(auditor.events)
		err := svc.ArchiveTeam(adminCtx(), created.ID)
		if err != nil {
			t.Fatalf("second ArchiveTeam failed: %v", err)
		}
		if len(archiver.archivedTeams) != 0 {
			t.Errorf("expected no additional project archives, got %v", archiver.archivedTeams)
		}
		if len(auditor.events) != eventCount {
			t.Errorf("expected no duplicate audit events")
		}
	})
}

func TestReplaceMembers(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	auditor := &mockAuditor{}
	svc := team.New(repo, nil, auditor, mockClock{now: now})

	created, _ := svc.CreateTeam(adminCtx(), "Security")

	t.Run("duplicate user ID rejected", func(t *testing.T) {
		err := svc.ReplaceMembers(adminCtx(), created.ID, []team.MembershipInput{
			{UserID: 10, Role: "lead"},
			{UserID: 10, Role: "member"},
		})
		if !errors.Is(err, team.ErrDuplicateMember) {
			t.Errorf("expected ErrDuplicateMember, got %v", err)
		}
	})

	t.Run("invalid role rejected", func(t *testing.T) {
		err := svc.ReplaceMembers(adminCtx(), created.ID, []team.MembershipInput{
			{UserID: 10, Role: "owner"},
		})
		if !errors.Is(err, team.ErrInvalidRole) {
			t.Errorf("expected ErrInvalidRole, got %v", err)
		}
	})

	t.Run("unknown team rejected", func(t *testing.T) {
		err := svc.ReplaceMembers(adminCtx(), 9999, []team.MembershipInput{
			{UserID: 10, Role: "member"},
		})
		if !errors.Is(err, team.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("success replaces members and audits", func(t *testing.T) {
		err := svc.ReplaceMembers(adminCtx(), created.ID, []team.MembershipInput{
			{UserID: 10, Role: "lead"},
			{UserID: 11, Role: "member"},
		})
		if err != nil {
			t.Fatalf("ReplaceMembers failed: %v", err)
		}
		if len(repo.memberships[created.ID]) != 2 {
			t.Fatalf("expected 2 members, got %d", len(repo.memberships[created.ID]))
		}
		if auditor.events[len(auditor.events)-1] != "team.members.replace" {
			t.Errorf("expected team.members.replace audit, got %v", auditor.events)
		}
	})
}

func TestListAndGetByID(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	svc := team.New(repo, nil, nil, mockClock{now: now})

	t1, _ := svc.CreateTeam(adminCtx(), "Frontend")
	t2, _ := svc.CreateTeam(adminCtx(), "Backend")

	t.Run("ListTeams permissions", func(t *testing.T) {
		// Member forbidden
		if _, err := svc.ListTeams(memberCtx(), false); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden for member, got %v", err)
		}
		// Admin allowed
		teams, err := svc.ListTeams(adminCtx(), false)
		if err != nil || len(teams) != 2 {
			t.Errorf("admin ListTeams failed: err=%v, count=%d", err, len(teams))
		}
		// Lead allowed
		teams, err = svc.ListTeams(leadCtx(t1.ID), false)
		if err != nil || len(teams) != 2 {
			t.Errorf("lead ListTeams failed: err=%v, count=%d", err, len(teams))
		}
	})

	t.Run("GetByID permissions", func(t *testing.T) {
		// Member forbidden
		if _, err := svc.GetByID(memberCtx(), t1.ID); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden for member, got %v", err)
		}
		// Lead of different team forbidden
		if _, err := svc.GetByID(leadCtx(t2.ID), t1.ID); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("expected ErrForbidden for lead of different team, got %v", err)
		}
		// Lead of this team allowed
		got, err := svc.GetByID(leadCtx(t1.ID), t1.ID)
		if err != nil || got.ID != t1.ID {
			t.Errorf("lead GetByID failed: err=%v, team=%+v", err, got)
		}
		// Admin allowed
		got, err = svc.GetByID(adminCtx(), t2.ID)
		if err != nil || got.ID != t2.ID {
			t.Errorf("admin GetByID failed: err=%v, team=%+v", err, got)
		}
	})
}
