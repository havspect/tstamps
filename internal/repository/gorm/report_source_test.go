package gorm

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/report"
)

func TestReportSourceScope(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	ws := &Workspace{Slug: "acme"}
	if err := db.WithContext(ctx).Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsCtx := WithWorkspaceID(ctx, ws.ID)

	lead := &User{Email: "lead@example.com", Name: "Lead", PasswordHash: "x"}
	other := &User{Email: "other@example.com", Name: "Other", PasswordHash: "x"}
	for _, u := range []*User{lead, other} {
		if err := db.WithContext(wsCtx).Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	ledTeam := &Team{Name: "Led"}
	otherTeam := &Team{Name: "Other"}
	for _, team := range []*Team{ledTeam, otherTeam} {
		if err := db.WithContext(wsCtx).Create(team).Error; err != nil {
			t.Fatalf("create team: %v", err)
		}
	}

	ledProject := &Project{TeamID: ledTeam.ID, Name: "Led project"}
	otherProject := &Project{TeamID: otherTeam.ID, Name: "Other project"}
	for _, p := range []*Project{ledProject, otherProject} {
		if err := db.WithContext(wsCtx).Create(p).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}
	}

	start := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	entries := []*Entry{
		{UserID: lead.ID, ProjectID: ledProject.ID},
		{UserID: other.ID, ProjectID: ledProject.ID},
		{UserID: lead.ID, ProjectID: otherProject.ID},
		{UserID: other.ID, ProjectID: otherProject.ID},
	}
	for _, e := range entries {
		e.StartAt, e.EndAt, e.Timezone = start, &end, "UTC"
		if err := db.WithContext(wsCtx).Create(e).Error; err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}

	source := NewReportSource(db)
	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	ids := func(in report.Input) []uint {
		out := make([]uint, len(in.Entries))
		for i, e := range in.Entries {
			out[i] = e.ID
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	// A lead reads their team's projects plus their own entries elsewhere, and
	// not another person's entries on a team they do not lead.
	in, err := source.LoadReportData(wsCtx, report.Filter{
		From: from, To: to, TeamIDs: []uint{ledTeam.ID}, SelfUserID: lead.ID,
	})
	if err != nil {
		t.Fatalf("load lead report: %v", err)
	}
	want := []uint{entries[0].ID, entries[1].ID, entries[2].ID}
	if got := ids(in); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("lead scope = %v, want %v", got, want)
	}

	// A member reads only their own entries, whichever project they sit on.
	in, err = source.LoadReportData(wsCtx, report.Filter{From: from, To: to, UserID: other.ID})
	if err != nil {
		t.Fatalf("load member report: %v", err)
	}
	want = []uint{entries[1].ID, entries[3].ID}
	if got := ids(in); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("member scope = %v, want %v", got, want)
	}
}
