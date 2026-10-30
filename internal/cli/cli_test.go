package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/client"
)

// fakeClient embeds the interface so a test implements only what its command
// calls; an unexpected call panics and fails the test loudly.
type fakeClient struct {
	Client
	me       client.Me
	projects []client.Project
	teams    []client.Team
	teamsErr error
	users    []client.User
	entries  []client.Entry
	report   client.Report
	csv      []byte
	tokens   []client.Token
	minted   *client.Token
	started  *client.Entry
	stopped  *client.Entry
	audit    []client.AuditEvent

	planIn   client.PlanCellInput
	seedIn   client.SeedPlanInput
	repeatIn client.RepeatPlanInput

	startErr   error
	deleted    uint
	revoked    uint
	entryIn    client.EntryInput
	entryQ     client.EntryQuery
	reportQ    client.ReportQuery
	projIn     client.ProjectInput
	entryPatch client.EntryPatch
	auditQ     client.AuditQuery
	auditErr   error

	deviceAuth *client.DeviceAuthResponse
	deviceErr  error
	tokenSet   *client.TokenSet
	pollErr    error
	refreshSet *client.TokenSet
	refreshErr error
	loginEmail string
	loginPass  string
	loginErr   error
}

func (f *fakeClient) Login(email, password string) (*client.Me, error) {
	f.loginEmail = email
	f.loginPass = password
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return &f.me, nil
}

func (f *fakeClient) StartDeviceAuth() (*client.DeviceAuthResponse, error) {
	return f.deviceAuth, f.deviceErr
}

func (f *fakeClient) PollDeviceToken(context.Context, string, time.Duration) (*client.TokenSet, error) {
	return f.tokenSet, f.pollErr
}

func (f *fakeClient) RefreshOAuthToken() (*client.TokenSet, error) {
	return f.refreshSet, f.refreshErr
}

func (f *fakeClient) Me() (*client.Me, error) { return &f.me, nil }

func (f *fakeClient) ListProjects(bool) ([]client.Project, error) { return f.projects, nil }

func (f *fakeClient) CreateProject(in client.ProjectInput) (*client.Project, error) {
	f.projIn = in
	return &client.Project{ID: 9, TeamID: in.TeamID, Name: in.Name}, nil
}

func (f *fakeClient) PatchProject(id uint, p client.ProjectPatch) (*client.Project, error) {
	archived := time.Now()
	if p.Archived != nil && !*p.Archived {
		archived = time.Time{}
	}
	return &client.Project{ID: id, Name: "Timeline", ArchivedAt: &archived}, nil
}

func (f *fakeClient) ListTeams() ([]client.Team, error) { return f.teams, f.teamsErr }

func (f *fakeClient) ListUsers() ([]client.User, error) { return f.users, nil }

func (f *fakeClient) ListEntries(q client.EntryQuery) ([]client.Entry, error) {
	f.entryQ = q
	return f.entries, nil
}

func (f *fakeClient) CreateEntry(in client.EntryInput) (*client.Entry, error) {
	f.entryIn = in
	e := in.EndAt
	return &client.Entry{ID: 1, ProjectID: in.ProjectID, StartAt: in.StartAt, EndAt: &e, Note: in.Note}, nil
}

func (f *fakeClient) PatchEntry(_ uint, p client.EntryPatch) (*client.Entry, error) {
	f.entryPatch = p
	e := p.EndAt
	return &client.Entry{ID: 1, ProjectID: 1, StartAt: *p.StartAt, EndAt: e}, nil
}

func (f *fakeClient) DeleteEntry(id uint) error { f.deleted = id; return nil }

func (f *fakeClient) StartTimer(_ uint, _ string, _ string) (*client.Entry, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.started, nil
}

func (f *fakeClient) StopTimer(string) (*client.Entry, error) { return f.stopped, nil }

func (f *fakeClient) Report(q client.ReportQuery) (*client.Report, error) {
	f.reportQ = q
	return &f.report, nil
}

func (f *fakeClient) ReportCSV(client.ReportQuery) ([]byte, error) { return f.csv, nil }

func (f *fakeClient) ListTokens() ([]client.Token, error) { return f.tokens, nil }

func (f *fakeClient) CreateToken(string) (*client.Token, error) { return f.minted, nil }

func (f *fakeClient) RevokeToken(id uint) error { f.revoked = id; return nil }

func (f *fakeClient) ListAudit(q client.AuditQuery) ([]client.AuditEvent, error) {
	f.auditQ = q
	return f.audit, f.auditErr
}

// testNow is a Tuesday afternoon: bare times land on 2026-10-20, and Berlin is
// two hours ahead of UTC, so a mistake in the zone shows up in the assertions.
var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func newTestApp(f Client) (*App, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	a := &App{
		Out: out, Err: errOut, In: strings.NewReader(""),
		NewClient: func(client.Config) Client { return f },
		LoadConfig: func(string, string) (client.Config, error) {
			return client.Config{URL: "http://tstamps.test", Token: "ts_test"}, nil
		},
		SaveConfig: func(client.Config) error { return nil },
		Now:        func() time.Time { return testNow },
	}
	return a, out, errOut
}

// berlinApp is newTestApp with a machine and user in Europe/Berlin, so local
// and UTC differ and a mistake in the zone shows up in the assertions.
func berlinApp(f *fakeClient) (*App, *bytes.Buffer, *bytes.Buffer) {
	f.me = client.Me{ID: 7, Email: "ada@example.com", Name: "Ada Lovelace", Timezone: "Europe/Berlin"}
	a, out, errOut := newTestApp(f)
	if loc, err := time.LoadLocation("Europe/Berlin"); err == nil {
		a.Local = loc
	}
	a.Zone = "Europe/Berlin"
	return a, out, errOut
}

func TestAddUsesMachineZone(t *testing.T) {
	f := &fakeClient{projects: []client.Project{{ID: 1, Name: "Timeline"}}}
	a, out, errOut := berlinApp(f)

	code := a.Run([]string{"add", "Timeline", "--from", "09:00", "--to", "11:30", "--note", "drag"})
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	wantStart := time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	if !f.entryIn.StartAt.Equal(wantStart) || !f.entryIn.EndAt.Equal(wantEnd) {
		t.Fatalf("entry %v-%v, want %v-%v", f.entryIn.StartAt, f.entryIn.EndAt, wantStart, wantEnd)
	}
	if f.entryIn.Note != "drag" || f.entryIn.ProjectID != 1 {
		t.Fatalf("entry %+v, want project 1 and the note", f.entryIn)
	}
	if f.entryIn.Timezone != "Europe/Berlin" {
		t.Fatalf("timezone = %q, want the machine zone", f.entryIn.Timezone)
	}
	if !strings.Contains(out.String(), "2h30m") || !strings.Contains(out.String(), "Timeline") {
		t.Fatalf("stdout %q does not summarise the entry", out.String())
	}
}

func TestAddEndBeforeStartCrossesMidnight(t *testing.T) {
	f := &fakeClient{projects: []client.Project{{ID: 1, Name: "Timeline"}}}
	a, _, errOut := berlinApp(f)

	if code := a.Run([]string{"add", "Timeline", "--from", "23:00", "--to", "01:00"}); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	wantEnd := time.Date(2026, 10, 21, 1, 0, 0, 0, time.FixedZone("", 2*3600))
	if !f.entryIn.EndAt.Equal(wantEnd) {
		t.Fatalf("end %v, want the next local midnight %v", f.entryIn.EndAt, wantEnd)
	}
}

func TestAddAcceptsDuration(t *testing.T) {
	f := &fakeClient{projects: []client.Project{{ID: 1, Name: "Timeline"}}}
	a, _, errOut := berlinApp(f)

	if code := a.Run([]string{"add", "Timeline", "--from", "09:00", "--to", "1h30m"}); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if got := f.entryIn.EndAt.Sub(f.entryIn.StartAt); got != 90*time.Minute {
		t.Fatalf("duration %v, want 1h30m", got)
	}
}

func TestProjectNameBeatsID(t *testing.T) {
	f := &fakeClient{projects: []client.Project{
		{ID: 1, Name: "Timeline"},
		{ID: 2, Name: "42"},
	}}
	a, _, errOut := berlinApp(f)

	if code := a.Run([]string{"add", "42", "--from", "09:00", "--to", "10:00"}); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if f.entryIn.ProjectID != 2 {
		t.Fatalf("project id %d, want the project named 42", f.entryIn.ProjectID)
	}
}

func TestStartNamesTheRunningEntry(t *testing.T) {
	f := &fakeClient{
		projects: []client.Project{{ID: 1, Name: "Timeline"}},
		startErr: &client.Error{
			StatusCode: 409, Type: "urn:tstamps:error:timer-already-running",
			RunningEntry: &client.Entry{ID: 3, ProjectID: 1,
				StartAt: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)},
		},
	}
	a, _, errOut := berlinApp(f)

	if code := a.Run([]string{"start", "Timeline"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if got := errOut.String(); !strings.Contains(got, "already running since 10:00 on Timeline") {
		t.Fatalf("stderr %q does not name the running entry in local time", got)
	}
}

func TestLsJSONAndUserMe(t *testing.T) {
	f := &fakeClient{entries: []client.Entry{{
		ID: 5, ProjectID: 1, UserID: 7,
		StartAt: time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC), Note: "snap",
	}}}
	a, out, errOut := berlinApp(f)

	if code := a.Run([]string{"ls", "--user", "me", "--json"}); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if f.entryQ.UserID != 7 {
		t.Fatalf("query user %d, want me (7)", f.entryQ.UserID)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", out.String(), err)
	}
	if len(got) != 1 || got[0]["note"] != "snap" {
		t.Fatalf("JSON %v, want the entry", got)
	}
}

func TestReportTableAndFilters(t *testing.T) {
	f := &fakeClient{
		me: client.Me{ID: 7, Email: "ada@example.com", Timezone: "Europe/Berlin"},
		report: client.Report{
			Rows: []client.ReportRow{{
				Bucket: "2026-W43", User: "Ada Lovelace", Team: "Platform",
				Project: "Timeline", Seconds: 9000,
			}},
			TotalSeconds: 9000,
		},
	}
	a, out, errOut := newTestApp(f)

	code := a.Run([]string{"report", "--from", "2026-10-19", "--to", "2026-10-26", "--by", "week", "--user", "me"})
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	if f.reportQ.From != "2026-10-19" || f.reportQ.To != "2026-10-26" || f.reportQ.By != "week" || f.reportQ.UserID != 7 {
		t.Fatalf("query %+v, want the filters passed through", f.reportQ)
	}
	for _, want := range []string{"2026-W43", "Ada Lovelace", "2h30m", "total"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout %q does not contain %q", out.String(), want)
		}
	}
}

func TestTokensCreatePrintsSecretOnce(t *testing.T) {
	f := &fakeClient{minted: &client.Token{ID: 4, Name: "laptop", Token: "ts_secret"}}
	a, out, _ := newTestApp(f)

	if code := a.Run([]string{"tokens", "create", "laptop"}); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(out.String(), "ts_secret") {
		t.Fatalf("stdout %q does not show the token", out.String())
	}
	if !strings.Contains(out.String(), "shown once") {
		t.Fatalf("stdout %q does not warn that the token is shown once", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	a, _, errOut := newTestApp(&fakeClient{})
	if code := a.Run([]string{"nope"}); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr %q does not name the problem", errOut.String())
	}
}

func TestMissingURL(t *testing.T) {
	a, _, errOut := newTestApp(&fakeClient{})
	a.LoadConfig = func(string, string) (client.Config, error) { return client.Config{}, nil }
	if code := a.Run([]string{"whoami"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "no server URL") {
		t.Fatalf("stderr %q does not explain the missing URL", errOut.String())
	}
}

// Echo is only disabled on a terminal; a piped or injected reader still works.
func TestReadPasswordFallsBackWhenNotATerminal(t *testing.T) {
	a := &App{In: strings.NewReader("secret\n"), Out: &bytes.Buffer{}}
	got, err := a.readPassword(bufio.NewReader(a.In))
	if err != nil || got != "secret" {
		t.Fatalf("readPassword = %q, %v, want secret", got, err)
	}
}

func (f *fakeClient) Plan(client.PlanQuery) (*client.PlanBoard, error) {
	return &client.PlanBoard{}, nil
}

func (f *fakeClient) SetPlan(in client.PlanCellInput) (*client.PlanCell, error) {
	f.planIn = in
	return &client.PlanCell{
		UserID: in.UserID, ProjectID: in.ProjectID, WeekStart: in.WeekStart, Hours: in.Hours,
	}, nil
}

func (f *fakeClient) SeedPlan(in client.SeedPlanInput) (*client.PlanSummary, error) {
	f.seedIn = in
	return &client.PlanSummary{Filled: 2, Skipped: 1}, nil
}

func (f *fakeClient) RepeatPlan(in client.RepeatPlanInput) (*client.PlanSummary, error) {
	f.repeatIn = in
	return &client.PlanSummary{Filled: 1}, nil
}

// TestPlanSetParsesAnISOWeek turns "2026-W46" into that week's Monday, which is
// what the API stores; a date in the middle of a week works too.
func TestPlanSetParsesAnISOWeek(t *testing.T) {
	f := &fakeClient{
		me:       client.Me{ID: 1, Email: "lead@example.com", IsAdmin: true},
		users:    []client.User{{ID: 2, Email: "ana@example.com", Name: "Ana"}},
		projects: []client.Project{{ID: 10, Name: "Timeline"}},
	}
	a, out, errOut := newTestApp(f)

	if code := a.Run([]string{"plan", "set", "ana@example.com", "Timeline", "--week", "2026-W46", "--hours", "20"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if f.planIn.WeekStart != "2026-11-09" || f.planIn.Hours != 20 || f.planIn.UserID != 2 || f.planIn.ProjectID != 10 {
		t.Fatalf("plan input = %+v", f.planIn)
	}
	if !strings.Contains(out.String(), "2026-W46") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPlanSeedRequiresBothMonths(t *testing.T) {
	f := &fakeClient{me: client.Me{ID: 1, IsAdmin: true}}
	a, _, errOut := newTestApp(f)

	if code := a.Run([]string{"plan", "seed", "--from", "2026-10"}); code != 2 {
		t.Fatalf("exit %d, want usage error 2", code)
	}
	if !strings.Contains(errOut.String(), "plan seed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}
