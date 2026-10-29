package report

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type mockSource struct {
	input  Input
	err    error
	filter Filter
}

func (m *mockSource) LoadReportData(ctx context.Context, filter Filter) (Input, error) {
	m.filter = filter
	if m.err != nil {
		return Input{}, m.err
	}
	return m.input, nil
}

func testContext(p *auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), p)
}

func testReportFixture() Input {
	start1 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	end1 := time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	end2 := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	return Input{
		Entries: []Entry{
			{ID: 1, UserID: 1, ProjectID: 1, StartAt: start1, EndAt: &end1, Timezone: "UTC", Note: "feature A"},
			{ID: 2, UserID: 1, ProjectID: 1, StartAt: start2, EndAt: &end2, Timezone: "UTC", Note: "feature B"},
		},
		Users: []User{
			{ID: 1, Name: "Ada Lovelace", Email: "ada@example.com"},
		},
		Projects: []Project{
			{ID: 1, TeamID: 1, Name: "Core Engine"},
		},
		Teams: []Team{
			{ID: 1, Name: "Platform"},
		},
		Now: testNow,
	}
}

func TestGenerateReport(t *testing.T) {
	source := &mockSource{input: testReportFixture()}
	svc := NewService(source, authz.NewPolicyEngine(), fixedClock{now: testNow})

	admin := &auth.Principal{WorkspaceID: 1, UserID: 99, Roles: []string{"admin"}}
	member := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"member"}}
	lead := &auth.Principal{WorkspaceID: 1, UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{1}}

	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	// 1. A member's own report ignores the team filter and narrows to self
	_, err := svc.GenerateReport(testContext(member), Filter{From: from, To: to, TeamID: 1})
	if err != nil {
		t.Fatalf("member GenerateReport: %v", err)
	}
	if source.filter.UserID != member.UserID || source.filter.TeamID != 0 {
		t.Errorf("member scope = user %d team %d, want user %d, no team", source.filter.UserID, source.filter.TeamID, member.UserID)
	}

	// 2. Lead can view their team's report
	rep, err := svc.GenerateReport(testContext(lead), Filter{From: from, To: to, By: "day", TeamID: 1})
	if err != nil {
		t.Fatalf("lead GenerateReport: %v", err)
	}
	if rep.TotalSeconds != 4*3600 {
		t.Errorf("expected 4 hours total, got %d", rep.TotalSeconds)
	}
	if len(rep.Rows) != 2 {
		t.Errorf("expected 2 day rows, got %d", len(rep.Rows))
	}
	if len(source.filter.TeamIDs) != 1 || source.filter.TeamIDs[0] != 1 || source.filter.SelfUserID != lead.UserID {
		t.Errorf("lead scope = teams %v self %d, want team 1 and self %d", source.filter.TeamIDs, source.filter.SelfUserID, lead.UserID)
	}

	// 3. Admin can view report with default week aggregation
	adminRep, err := svc.GenerateReport(testContext(admin), Filter{From: from, To: to})
	if err != nil {
		t.Fatalf("admin GenerateReport: %v", err)
	}
	if len(adminRep.Rows) != 1 || adminRep.Rows[0].Bucket != "2026-W43" {
		t.Errorf("expected 1 row in 2026-W43, got %+v", adminRep.Rows)
	}

	// 4. Source error propagated
	source.err = errors.New("db error")
	_, err = svc.GenerateReport(testContext(admin), Filter{From: from, To: to})
	if err == nil || !strings.Contains(err.Error(), "db error") {
		t.Errorf("expected db error, got %v", err)
	}
}

func TestExportCSV(t *testing.T) {
	source := &mockSource{input: testReportFixture()}
	svc := NewService(source, authz.NewPolicyEngine(), fixedClock{now: testNow})

	admin := &auth.Principal{WorkspaceID: 1, UserID: 99, Roles: []string{"admin"}}
	member := &auth.Principal{WorkspaceID: 1, UserID: 1, Roles: []string{"member"}}

	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	// 1. A member exports their own hours
	var buf bytes.Buffer
	err := svc.ExportCSV(testContext(member), Filter{From: from, To: to}, &buf)
	if err != nil {
		t.Fatalf("member ExportCSV: %v", err)
	}
	if source.filter.UserID != member.UserID {
		t.Errorf("member CSV scope = user %d, want user %d", source.filter.UserID, member.UserID)
	}

	// 2. Admin exports CSV
	buf.Reset()
	err = svc.ExportCSV(testContext(admin), Filter{From: from, To: to}, &buf)
	if err != nil {
		t.Fatalf("admin ExportCSV: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	// 1 header + 2 rows = 3 records
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d: %s", len(records), buf.String())
	}
	if records[0][0] != "date" || records[1][0] != "2026-10-19" || records[2][0] != "2026-10-20" {
		t.Errorf("unexpected dates in CSV: %+v", records)
	}
}
