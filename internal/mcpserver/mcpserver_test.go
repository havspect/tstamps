package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

type mockUserService struct {
	profile    *user.Profile
	profileErr error
	users      []user.User
	usersErr   error
}

func (m *mockUserService) GetProfile(ctx context.Context) (*user.Profile, error) {
	return m.profile, m.profileErr
}

func (m *mockUserService) ListUsers(ctx context.Context) ([]user.User, error) {
	return m.users, m.usersErr
}

type mockProjectService struct {
	projects        []project.Project
	err             error
	lastAllProjects bool
}

func (m *mockProjectService) ListProjects(ctx context.Context, includeArchived bool) ([]project.Project, error) {
	m.lastAllProjects = includeArchived
	return m.projects, m.err
}

type mockTeamService struct {
	teams []team.TeamWithMembers
	err   error
}

func (m *mockTeamService) ListTeams(ctx context.Context, includeArchived bool) ([]team.TeamWithMembers, error) {
	return m.teams, m.err
}

type mockEntryService struct {
	entries       []entry.Entry
	listErr       error
	lastFilter    entry.ListFilter
	startEntry    *entry.Entry
	startErr      error
	stopEntry     *entry.Entry
	stopErr       error
	logEntry      *entry.Entry
	logErr        error
	patchEntry    *entry.Entry
	patchErr      error
	deleteErr     error
	lastDeletedID uint
}

func (m *mockEntryService) ListEntries(ctx context.Context, filter entry.ListFilter) ([]entry.Entry, error) {
	m.lastFilter = filter
	return m.entries, m.listErr
}

func (m *mockEntryService) StartTimer(ctx context.Context, projectID uint, tz, note string) (*entry.Entry, error) {
	if m.startErr != nil {
		return nil, m.startErr
	}
	if m.startEntry != nil {
		return m.startEntry, nil
	}
	return &entry.Entry{ID: 1, ProjectID: projectID, Timezone: tz, Note: note}, nil
}

func (m *mockEntryService) StopTimer(ctx context.Context, note *string) (*entry.Entry, error) {
	if m.stopErr != nil {
		return nil, m.stopErr
	}
	if m.stopEntry != nil {
		return m.stopEntry, nil
	}
	n := ""
	if note != nil {
		n = *note
	}
	now := time.Now()
	return &entry.Entry{ID: 1, EndAt: &now, Note: n}, nil
}

func (m *mockEntryService) LogEntry(ctx context.Context, projectID uint, start, end time.Time, tz, note string) (*entry.Entry, error) {
	if m.logErr != nil {
		return nil, m.logErr
	}
	if m.logEntry != nil {
		return m.logEntry, nil
	}
	return &entry.Entry{ID: 2, ProjectID: projectID, StartAt: start, EndAt: &end, Timezone: tz, Note: note}, nil
}

func (m *mockEntryService) PatchEntry(ctx context.Context, id uint, projectID *uint, start, end *time.Time, timezone *string, note *string) (*entry.Entry, error) {
	if m.patchErr != nil {
		return nil, m.patchErr
	}
	if m.patchEntry != nil {
		return m.patchEntry, nil
	}
	return &entry.Entry{ID: id}, nil
}

func (m *mockEntryService) DeleteEntry(ctx context.Context, id uint) error {
	m.lastDeletedID = id
	return m.deleteErr
}

type mockReportService struct {
	report     *report.Report
	err        error
	lastFilter report.Filter
}

func (m *mockReportService) GenerateReport(ctx context.Context, filter report.Filter) (*report.Report, error) {
	m.lastFilter = filter
	return m.report, m.err
}

func connectTestClient(t *testing.T, cfg Config) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := NewServer(cfg)
	t1, t2 := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1.0"}, nil).Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestToolsCatalogueAndAnnotations(t *testing.T) {
	cs := connectTestClient(t, Config{})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	wantReadOnly := map[string]bool{
		"whoami":        true,
		"list_projects": true,
		"list_teams":    true,
		"list_users":    true,
		"list_entries":  true,
		"report":        true,
		"get_plan":      true,
		"set_plan":      false,
		"seed_plan":     false,
		"repeat_plan":   false,
		"start_timer":   false,
		"stop_timer":    false,
		"add_entry":     false,
		"update_entry":  false,
		"delete_entry":  false,
	}

	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q missing description", tool.Name)
		}
		got[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}

	for name, wantRO := range wantReadOnly {
		flag, ok := got[name]
		if !ok {
			t.Errorf("missing tool %q", name)
			continue
		}
		if flag != wantRO {
			t.Errorf("tool %q ReadOnlyHint=%v, want %v", name, flag, wantRO)
		}
	}
	if len(got) != len(wantReadOnly) {
		t.Errorf("got %d tools, want %d", len(got), len(wantReadOnly))
	}
}

func TestWhoami(t *testing.T) {
	uSvc := &mockUserService{
		profile: &user.Profile{
			User: user.User{
				ID:       42,
				Email:    "ada@example.com",
				Name:     "Ada",
				Timezone: "Europe/London",
				IsAdmin:  true,
			},
			LedTeamIDs: []uint{1, 2},
		},
	}
	cs := connectTestClient(t, Config{UserService: uSvc})

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("CallTool whoami: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content in result")
	}

	raw, _ := json.Marshal(res.Content[0])
	var txt struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(raw, &txt)

	var me Me
	if err := json.Unmarshal([]byte(txt.Text), &me); err != nil {
		t.Fatalf("unmarshal whoami: %v, raw text: %s", err, txt.Text)
	}
	if me.ID != 42 || me.Email != "ada@example.com" || !me.IsAdmin {
		t.Errorf("unexpected me: %+v", me)
	}
	if len(me.Memberships) != 2 {
		t.Errorf("expected 2 memberships, got %d", len(me.Memberships))
	}
}

func TestListProjects(t *testing.T) {
	pSvc := &mockProjectService{
		projects: []project.Project{
			{ID: 10, TeamID: 1, Name: "Alpha", Color: "#ff0000"},
		},
	}
	cs := connectTestClient(t, Config{ProjectService: pSvc})

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"all": true},
	})
	if err != nil {
		t.Fatalf("CallTool list_projects: %v", err)
	}
	if !pSvc.lastAllProjects {
		t.Errorf("expected includeArchived=true")
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content")
	}
}

func TestListTeams(t *testing.T) {
	tSvc := &mockTeamService{
		teams: []team.TeamWithMembers{
			{Team: team.Team{ID: 1, Name: "Dev"}, Members: []team.Membership{{UserID: 5, Role: "lead"}}},
		},
	}
	cs := connectTestClient(t, Config{TeamService: tSvc})

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "list_teams"})
	if err != nil {
		t.Fatalf("CallTool list_teams: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content")
	}
}

func TestListUsers(t *testing.T) {
	uSvc := &mockUserService{
		users: []user.User{
			{ID: 1, Email: "user@example.com", Name: "User 1", Active: true},
		},
	}
	cs := connectTestClient(t, Config{UserService: uSvc})

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "list_users"})
	if err != nil {
		t.Fatalf("CallTool list_users: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content")
	}
}

func TestEntryOperations(t *testing.T) {
	eSvc := &mockEntryService{}
	uSvc := &mockUserService{
		profile: &user.Profile{
			User: user.User{ID: 1, Timezone: "America/New_York"},
		},
	}
	cs := connectTestClient(t, Config{EntryService: eSvc, UserService: uSvc})

	// 1. start_timer
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "start_timer",
		Arguments: map[string]any{"project_id": 5, "note": "working"},
	})
	if err != nil {
		t.Fatalf("start_timer: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("start_timer expected content")
	}

	// 2. stop_timer
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "stop_timer",
		Arguments: map[string]any{"note": "done"},
	})
	if err != nil {
		t.Fatalf("stop_timer: %v", err)
	}

	// 3. add_entry
	start := time.Now().Add(-time.Hour).Format(time.RFC3339)
	end := time.Now().Format(time.RFC3339)
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "add_entry",
		Arguments: map[string]any{"project_id": 5, "start_at": start, "end_at": end, "note": "test entry"},
	})
	if err != nil {
		t.Fatalf("add_entry: %v", err)
	}

	// 4. update_entry
	note := "updated"
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "update_entry",
		Arguments: map[string]any{"id": 10, "note": note},
	})
	if err != nil {
		t.Fatalf("update_entry: %v", err)
	}

	// 5. delete_entry
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "delete_entry",
		Arguments: map[string]any{"id": 10},
	})
	if err != nil {
		t.Fatalf("delete_entry: %v", err)
	}
	if eSvc.lastDeletedID != 10 {
		t.Errorf("delete_entry ID=%d, want 10", eSvc.lastDeletedID)
	}

	// 6. list_entries
	res, err = cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "list_entries",
		Arguments: map[string]any{"from": "2026-10-01", "to": "2026-10-07", "project_id": 5},
	})
	if err != nil {
		t.Fatalf("list_entries: %v", err)
	}
	if eSvc.lastFilter.ProjectID != 5 {
		t.Errorf("list_entries filter project ID=%d, want 5", eSvc.lastFilter.ProjectID)
	}
}

func TestReportTool(t *testing.T) {
	rSvc := &mockReportService{
		report: &report.Report{
			Rows: []report.Row{
				{Bucket: "2026-W42", UserID: 1, User: "Ada", Seconds: 3600},
			},
			TotalSeconds: 3600,
		},
	}
	cs := connectTestClient(t, Config{ReportService: rSvc})

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "report",
		Arguments: map[string]any{"from": "2026-10-01", "to": "2026-10-07", "by": "week"},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if rSvc.lastFilter.By != "week" {
		t.Errorf("report filter by=%s, want week", rSvc.lastFilter.By)
	}
	if len(res.Content) == 0 {
		t.Fatalf("report expected content")
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(Config{})
	if handler == nil {
		t.Fatalf("expected non-nil handler")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	handler.ServeHTTP(rec, req)
	// Streamable HTTP handler answers or requires session
	if rec.Code == 0 {
		t.Errorf("handler did not write response")
	}
}
