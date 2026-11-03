package gorm

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/entry"
)

// TestListEntriesLeadScope covers issue 24: a lead reads the projects of the
// teams they lead plus their own entries, and a caller filter cannot widen
// that scope.
func TestListEntriesLeadScope(t *testing.T) {
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
	ownLed := &Entry{UserID: lead.ID, ProjectID: ledProject.ID}
	foreignUnled := &Entry{UserID: other.ID, ProjectID: otherProject.ID}
	foreignLed := &Entry{UserID: other.ID, ProjectID: ledProject.ID}
	ownUnled := &Entry{UserID: lead.ID, ProjectID: otherProject.ID}
	for _, e := range []*Entry{ownLed, foreignUnled, foreignLed, ownUnled} {
		e.StartAt, e.EndAt, e.Timezone = start, &end, "UTC"
		if err := db.WithContext(wsCtx).Create(e).Error; err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}

	svc := entry.NewService(NewEntryRepository(db), nil, nil, nil, authz.NewPolicyEngine(), nil)
	leadPrincipal := &auth.Principal{
		WorkspaceID: ws.ID,
		UserID:      lead.ID,
		Roles:       []string{"member", "lead"},
		LedTeamIDs:  []uint{ledTeam.ID},
	}
	memberPrincipal := &auth.Principal{
		WorkspaceID: ws.ID,
		UserID:      lead.ID,
		Roles:       []string{"member"},
	}

	cases := []struct {
		name      string
		principal *auth.Principal
		filter    entry.ListFilter
		want      []uint
	}{
		{
			name:      "lead sees led projects and own entries",
			principal: leadPrincipal,
			want:      []uint{ownLed.ID, foreignLed.ID, ownUnled.ID},
		},
		{
			name:      "lead cannot widen to another user",
			principal: leadPrincipal,
			filter:    entry.ListFilter{UserID: other.ID},
			want:      []uint{foreignLed.ID},
		},
		{
			name:      "lead keeps own entries on an unled project",
			principal: leadPrincipal,
			filter:    entry.ListFilter{ProjectID: otherProject.ID},
			want:      []uint{ownUnled.ID},
		},
		{
			name:      "member sees own entries only",
			principal: memberPrincipal,
			want:      []uint{ownLed.ID, ownUnled.ID},
		},
		{
			name:      "member cannot widen to another user",
			principal: memberPrincipal,
			filter:    entry.ListFilter{UserID: other.ID},
			want:      []uint{ownLed.ID, ownUnled.ID},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := svc.ListEntries(auth.WithPrincipal(wsCtx, tc.principal), tc.filter)
			if err != nil {
				t.Fatalf("ListEntries: %v", err)
			}
			got := make([]uint, len(rows))
			for i, e := range rows {
				got[i] = e.ID
			}
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			sort.Slice(tc.want, func(i, j int) bool { return tc.want[i] < tc.want[j] })
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("entry IDs = %v, want %v", got, tc.want)
			}
		})
	}
}
