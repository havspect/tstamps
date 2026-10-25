package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestUserCLI walks every command of the user CLI against a real server. Where
// TestEndToEnd proves the v1 acceptance path, this keeps the whole CLI surface
// honest: listings, edits, deletes, filters, tokens and the error exit codes.
func TestUserCLI(t *testing.T) {
	dir := t.TempDir()
	serverBin := build(t, dir, "tstamps-server", "../cmd/tstamps-server")
	cliBin := build(t, dir, "tstamps", "../cmd/tstamps")

	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	// TZ=UTC keeps bare clock times and the report range deterministic: the
	// bootstrap admin is in UTC too.
	env := append(os.Environ(), "HOME="+home, "TZ=UTC", "TSTAMPS_URL=", "TSTAMPS_TOKEN=", "TSTAMPS_DB=")
	db := filepath.Join(dir, "tstamps.db")

	cli := func(args ...string) (string, int) {
		t.Helper()
		return runCommand(t, env, cliBin, args...)
	}
	must := func(args ...string) string {
		t.Helper()
		out, code := cli(args...)
		if code != 0 {
			t.Fatalf("tstamps %s: exit %d\n%s", strings.Join(args, " "), code, out)
		}
		return out
	}

	// The binary's own commands and its refusal to become the server.
	if out := must("version"); !strings.HasPrefix(out, "tstamps ") {
		t.Fatalf("version printed %q", out)
	}
	if out := must("help"); !strings.Contains(out, "Exit codes") || !strings.Contains(out, "tstamps login") {
		t.Fatalf("help printed %q", out)
	}
	assertExit(t, cli, 2, "unknown command", "nope")
	assertExit(t, cli, 2, "tstamps-server", "serve")
	assertExit(t, cli, 2, "usage: tstamps")

	adminPassword := passwordFrom(t, mustWith(t, env, serverBin, "admin", "create", "--db", db,
		"--email", "admin@example.com", "--name", "Ada Admin"))
	url, stopServer := startServer(t, serverBin, db)
	defer stopServer()

	// Teams, users and memberships are admin screens in v1; the CLI reads them.
	login := func() string {
		t.Helper()
		out := must("login", "--url", url, "--email", "admin@example.com", "--password", adminPassword)
		if !strings.Contains(out, "logged in as Ada Admin <admin@example.com>") {
			t.Fatalf("login printed %q", out)
		}
		return savedToken(t, home)
	}
	token := login()

	var me struct{ ID uint }
	apiCall(t, url, token, "GET", "/me", nil, &me)

	platform := createTeam(t, url, token, "Platform")
	mobile := createTeam(t, url, token, "Mobile")
	leadA := createUser(t, url, token, "grace@example.com", "Grace Hopper")
	leadB := createUser(t, url, token, "hedy@example.com", "Hedy Lamarr")
	memberUser := createUser(t, url, token, "linus@example.com", "Linus Torvalds")
	setMembers(t, url, token, platform.ID, []memberRole{
		{me.ID, "lead"}, {leadA.ID, "lead"}, {memberUser.ID, "member"},
	})
	setMembers(t, url, token, mobile.ID, []memberRole{{leadB.ID, "lead"}})

	// whoami names the user, the admin bit and every membership.
	if out := must("whoami"); !strings.Contains(out, "Ada Admin <admin@example.com>") ||
		!strings.Contains(out, "admin") || !strings.Contains(out, "lead of Platform") {
		t.Fatalf("whoami printed %q", out)
	}

	// Projects: create with a colour, list as a table and as JSON.
	addTimeline := must("projects", "add", "Timeline", "--team", "Platform", "--color", "#4f46e5")
	if !strings.Contains(addTimeline, "Timeline (Platform)") {
		t.Fatalf("projects add printed %q", addTimeline)
	}
	timelineID := leadingID(t, addTimeline, "created project ")
	addOps := must("projects", "add", "Ops", "--team", "Mobile")
	opsID := leadingID(t, addOps, "created project ")

	out := must("projects")
	if line := row(out, "Timeline"); !strings.Contains(line, "Platform") || !strings.Contains(line, "active") {
		t.Fatalf("projects table row %q", line)
	}
	projectsJSON := must("projects", "--json")
	for _, want := range []string{`"name": "Timeline"`, `"color": "#4f46e5"`, fmt.Sprintf(`"team_id": %d`, platform.ID)} {
		if !strings.Contains(projectsJSON, want) {
			t.Fatalf("projects --json %s does not contain %s", projectsJSON, want)
		}
	}

	// Teams: the table, the membership list, and the admin JSON reply.
	out = must("teams")
	if line := row(out, "Platform"); !strings.Contains(line, "active") || !strings.Contains(line, "3") {
		t.Fatalf("teams table row %q", line)
	}
	out = must("teams", "members", "Platform")
	if line := row(out, "Grace Hopper"); !strings.Contains(line, "<grace@example.com>") || !strings.Contains(line, "lead") {
		t.Fatalf("teams members row %q", line)
	}
	if line := row(out, "Linus Torvalds"); !strings.Contains(line, "member") {
		t.Fatalf("teams members row %q", line)
	}
	if out := must("teams", "--json"); !strings.Contains(out, `"name": "Platform"`) || !strings.Contains(out, `"members": [`) {
		t.Fatalf("teams --json printed %q", out)
	}

	// Entries in last week, so the range is fixed whatever day the test runs.
	now := time.Now().UTC()
	thisMonday := time.Date(now.Year(), now.Month(), now.Day()-((int(now.Weekday())+6)%7), 0, 0, 0, 0, time.UTC)
	lastMonday := thisMonday.AddDate(0, 0, -7)
	from, to := lastMonday.Format("2006-01-02"), thisMonday.Format("2006-01-02")

	if out := must("add", "Timeline", "--date", from, "--from", "09:00", "--to", "11:30", "--note", "drag, resize"); !strings.Contains(out, "2h30m") {
		t.Fatalf("add printed %q, want the 2h30m entry", out)
	}
	if out := must("add", "Ops", "--date", lastMonday.AddDate(0, 0, 1).Format("2006-01-02"), "--from", "13:00", "--to", "14:00", "--note", "deploy"); !strings.Contains(out, "1h0m") {
		t.Fatalf("add printed %q, want the 1h entry", out)
	}

	// ls as a table and as JSON, with the timezone the entry was logged in.
	out = must("ls", "--from", from, "--to", to)
	for _, want := range []string{"drag, resize", "deploy", "Timeline", "Ops", "2h30m", "1h0m"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls output %q does not contain %q", out, want)
		}
	}
	listed := decodeEntries(t, must("ls", "--from", from, "--to", to, "--json"))
	if len(listed) != 2 {
		t.Fatalf("ls --json has %d entries, want 2", len(listed))
	}
	timelineEntry := entryWithNote(t, listed, "drag, resize")
	wantStart := time.Date(lastMonday.Year(), lastMonday.Month(), lastMonday.Day(), 9, 0, 0, 0, time.UTC)
	if !timelineEntry.StartAt.Equal(wantStart) || timelineEntry.EndAt == nil || timelineEntry.EndAt.Sub(wantStart) != 150*time.Minute {
		t.Fatalf("Timeline entry %v-%v, want %v + 2h30m", timelineEntry.StartAt, timelineEntry.EndAt, wantStart)
	}
	if timelineEntry.Timezone != "UTC" {
		t.Fatalf("timezone = %q, want UTC", timelineEntry.Timezone)
	}
	opsEntry := entryWithNote(t, listed, "deploy")

	if got := decodeEntries(t, must("ls", "--user", "me", "--from", from, "--to", to, "--json")); len(got) != 2 {
		t.Fatalf("ls --user me has %d entries, want 2", len(got))
	}
	if got := decodeEntries(t, must("ls", "--user", "grace@example.com", "--from", from, "--to", to, "--json")); len(got) != 0 {
		t.Fatalf("ls --user grace has %d entries, want none", len(got))
	}
	if got := decodeEntries(t, must("ls", "--project", "Timeline", "--from", from, "--to", to, "--json")); len(got) != 1 || got[0].ProjectID != timelineID {
		t.Fatalf("ls --project Timeline = %+v, want the logged entry", got)
	}

	// edit retimes and annotates the entry, keeping it in range.
	if out := must("edit", strconv.FormatUint(uint64(timelineEntry.ID), 10),
		"--date", from, "--from", "09:00", "--to", "10:30", "--note", "edited note"); !strings.Contains(out, "updated entry") {
		t.Fatalf("edit printed %q", out)
	}
	edited := decodeEntries(t, must("ls", "--project", "Timeline", "--from", from, "--to", to, "--json"))
	if len(edited) != 1 || edited[0].Note != "edited note" || edited[0].EndAt == nil ||
		edited[0].EndAt.Sub(edited[0].StartAt) != 90*time.Minute {
		t.Fatalf("edited entry = %+v, want a 1h30m entry with the new note", edited)
	}

	// edit can correct the zone an entry was logged in.
	must("edit", strconv.FormatUint(uint64(opsEntry.ID), 10), "--timezone", "Europe/Berlin", "--note", "deploy (Berlin)")
	ops := decodeEntries(t, must("ls", "--project", "Ops", "--from", from, "--to", to, "--json"))
	if len(ops) != 1 || ops[0].Timezone != "Europe/Berlin" || ops[0].Note != "deploy (Berlin)" ||
		!ops[0].StartAt.Equal(opsEntry.StartAt) {
		t.Fatalf("Ops entry = %+v, want the Berlin zone and the new note at the same instant", ops)
	}

	// report: table, JSON, CSV and every filter.
	out = must("report", "--from", from, "--to", to, "--by", "week")
	for _, want := range []string{"BUCKET", "Timeline", "Ops", "1h30m", "1h0m", "total", "2h30m"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report output %q does not contain %q", out, want)
		}
	}
	totalOf := func(args ...string) int64 {
		t.Helper()
		var r struct {
			TotalSeconds int64 `json:"total_seconds"`
		}
		if err := json.Unmarshal([]byte(must(args...)), &r); err != nil {
			t.Fatalf("report %v is not JSON: %v", args, err)
		}
		return r.TotalSeconds
	}
	if got := totalOf("report", "--from", from, "--to", to, "--by", "week", "--json"); got != 9000 {
		t.Fatalf("report total = %d, want 9000", got)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--project", "Ops", "--json"); got != 3600 {
		t.Fatalf("report --project total = %d, want 3600", got)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--team", "Mobile", "--json"); got != 3600 {
		t.Fatalf("report --team Mobile total = %d, want 3600", got)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--user", "me", "--json"); got != 9000 {
		t.Fatalf("report --user me total = %d, want 9000", got)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--team", "Platform", "--json"); got != 5400 {
		t.Fatalf("report --team Platform = %d, want 5400", got)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--user", "grace@example.com", "--json"); got != 0 {
		t.Fatalf("report for a user without entries = %d, want 0", got)
	}
	if rows := reportRows(t, must("report", "--from", from, "--to", to, "--by", "day", "--json")); len(rows) != 2 {
		t.Fatalf("report --by day has %d rows, want one per day", len(rows))
	}
	if rows := reportRows(t, must("report", "--from", from, "--to", to, "--by", "none", "--json")); len(rows) != 2 {
		t.Fatalf("report --by none = %+v, want one row per project", rows)
	} else if rows[0].Bucket != "" || rows[1].Bucket != "" {
		t.Fatalf("report --by none = %+v, want unlabelled rows", rows)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--by", "month", "--json"); got != 9000 {
		t.Fatalf("report --by month = %d, want 9000", got)
	}
	csv := must("report", "--from", from, "--to", to, "--by", "week", "--csv")
	if !strings.Contains(csv, "date,iso_week,user,email,team,project,timezone,start,end,seconds,note") ||
		!strings.Contains(csv, ",5400,") || !strings.Contains(csv, ",3600,") || !strings.Contains(csv, "edited note") {
		t.Fatalf("CSV does not carry the entries:\n%s", csv)
	}
	assertExit(t, cli, 1, "invalid --by", "report", "--from", from, "--to", to, "--by", "fortnight")
	assertExit(t, cli, 1, "mutually exclusive", "report", "--from", from, "--to", to, "--json", "--csv")
	assertExit(t, cli, 1, "unknown user", "report", "--from", from, "--to", to, "--user", "nobody@example.com")

	// The timer runs from start to stop, and reports the running entry once.
	if out := must("start", "Timeline", "--note", "running"); !strings.Contains(out, "started") {
		t.Fatalf("start printed %q", out)
	}
	assertExit(t, cli, 1, "already running", "start", "Timeline")
	if out := must("stop", "--note", "done"); !strings.Contains(out, "stopped") || !strings.Contains(out, "Timeline") {
		t.Fatalf("stop printed %q", out)
	}
	current := decodeEntries(t, must("ls", "--json"))
	if len(current) != 1 || current[0].Note != "done" || current[0].EndAt == nil {
		t.Fatalf("ls shows %+v, want the stopped entry", current)
	}
	if out := must("rm", strconv.FormatUint(uint64(current[0].ID), 10)); !strings.Contains(out, "deleted entry") {
		t.Fatalf("rm printed %q", out)
	}
	if got := decodeEntries(t, must("ls", "--json")); len(got) != 0 {
		t.Fatalf("ls still shows %+v after rm", got)
	}

	// Deleting an entry takes it out of the report; the project can then be
	// archived, after which only --all lists it.
	if out := must("rm", strconv.FormatUint(uint64(opsEntry.ID), 10)); !strings.Contains(out, "deleted entry") {
		t.Fatalf("rm printed %q", out)
	}
	if got := totalOf("report", "--from", from, "--to", to, "--project", "Ops", "--json"); got != 0 {
		t.Fatalf("report for the deleted entry = %d, want 0", got)
	}
	assertExit(t, cli, 1, "Not Found", "rm", "999999")

	if out := must("projects", "archive", strconv.FormatUint(uint64(opsID), 10)); !strings.Contains(out, "archived project") {
		t.Fatalf("projects archive printed %q", out)
	}
	if out := must("projects"); row(out, "Ops") != "" {
		t.Fatalf("projects still lists the archived project:\n%s", out)
	}
	if line := row(must("projects", "--all"), "Ops"); !strings.Contains(line, "archived") || !strings.Contains(line, "Mobile") {
		t.Fatalf("projects --all row %q", line)
	}
	if out := must("projects", "--all", "--json"); !strings.Contains(out, `"archived_at": "`) || !strings.Contains(out, `"name": "Ops"`) {
		t.Fatalf("projects --all --json printed %q", out)
	}

	// The audit log records the admin's project archive (as project.update) and
	// nothing the owner did to their own entries.
	if out := must("audit"); !strings.Contains(out, "project.update") || !strings.Contains(out, "admin@example.com") {
		t.Fatalf("audit table printed %q", out)
	}
	archived := must("audit", "--action", "project.update", "--json")
	if !strings.Contains(archived, `"action": "project.update"`) || !strings.Contains(archived, `"actor_email": "admin@example.com"`) || !strings.Contains(archived, `"archived_at": "`) {
		t.Fatalf("audit --action project.update printed %q", archived)
	}
	if out := must("audit", "--action", "entry.delete", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("audit --action entry.delete printed %q, want []", out)
	}

	// Personal tokens: create, use without the stored config, list and revoke.
	created := must("tokens", "create", "laptop")
	tokenID := leadingID(t, created, "created token ")
	lines := strings.Split(strings.TrimSpace(created), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[1], "ts_") || !strings.Contains(created, "shown once") {
		t.Fatalf("tokens create printed %q", created)
	}
	secret := lines[1]

	bareHome := t.TempDir()
	bare := withHome(env, bareHome)
	if out := mustWith(t, bare, cliBin, "whoami", "--url", url, "--token", secret); !strings.Contains(out, "Ada Admin <admin@example.com>") {
		t.Fatalf("whoami with --token printed %q", out)
	}
	fromEnv := append(withHome(env, bareHome), "TSTAMPS_URL="+url, "TSTAMPS_TOKEN="+secret)
	if out := mustWith(t, fromEnv, cliBin, "whoami"); !strings.Contains(out, "Ada Admin <admin@example.com>") {
		t.Fatalf("whoami from TSTAMPS_URL/TSTAMPS_TOKEN printed %q", out)
	}
	if out := must("tokens", "ls"); !strings.Contains(row(out, "laptop"), "active") || !strings.Contains(out, "tstamps login") {
		t.Fatalf("tokens ls printed %q", out)
	}
	if out := must("tokens", "rm", strconv.FormatUint(uint64(tokenID), 10)); !strings.Contains(out, "revoked token") {
		t.Fatalf("tokens rm printed %q", out)
	}
	if out := must("tokens", "ls"); !strings.Contains(row(out, "laptop"), "revoked") {
		t.Fatalf("tokens ls after rm printed %q", out)
	}
	assertExit(t, cli, 1, "Unauthorized", "whoami", "--url", url, "--token", secret)
	if out := must("whoami"); !strings.Contains(out, "Ada Admin") {
		t.Fatalf("revoking another token broke the stored login: %q", out)
	}

	// A failed login must not replace the token that already works.
	assertExit(t, cli, 1, "tstamps login:", "login", "--url", url, "--email", "admin@example.com", "--password", "wrong")
	if out := must("whoami"); !strings.Contains(out, "Ada Admin") {
		t.Fatalf("failed login changed the stored token: %q", out)
	}

	// Usage and lookup errors keep their exit codes: 2 for usage, 1 for data.
	assertExit(t, cli, 2, "usage: tstamps add", "add", "Timeline", "--from", "09:00")
	assertExit(t, cli, 2, "usage: tstamps edit", "edit", "1")
	assertExit(t, cli, 2, "usage: tstamps ls", "ls", "extra")
	assertExit(t, cli, 2, "usage: tstamps tokens", "tokens")
	assertExit(t, cli, 1, "unknown project", "add", "Nope", "--from", "09:00", "--to", "10:00")
	assertExit(t, cli, 1, "invalid id", "rm", "abc")
}

// runCommand runs one binary with env and returns its combined output and exit
// code, so a test can assert the failures too.
func runCommand(t *testing.T, env []string, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run %s %s: %v", bin, strings.Join(args, " "), err)
	}
	return string(out), exitErr.ExitCode()
}

func mustWith(t *testing.T, env []string, bin string, args ...string) string {
	t.Helper()
	out, code := runCommand(t, env, bin, args...)
	if code != 0 {
		t.Fatalf("%s %s: exit %d\n%s", filepath.Base(bin), strings.Join(args, " "), code, out)
	}
	return out
}

// assertExit runs a command that must fail with code, and whose output names
// the problem.
func assertExit(t *testing.T, cli func(...string) (string, int), wantCode int, want string, args ...string) {
	t.Helper()
	out, code := cli(args...)
	if code != wantCode {
		t.Fatalf("tstamps %s: exit %d, want %d\n%s", strings.Join(args, " "), code, wantCode, out)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("tstamps %s printed %q, want it to mention %q", strings.Join(args, " "), out, want)
	}
}

// row returns the line holding substr, for table assertions.
func row(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

// leadingID reads the first number after marker, e.g. "created project 4:".
func leadingID(t *testing.T, out, marker string) uint {
	t.Helper()
	i := strings.Index(out, marker)
	if i < 0 {
		t.Fatalf("output %q has no %q", out, marker)
	}
	s := strings.TrimSpace(out[i+len(marker):])
	if j := strings.IndexAny(s, ": \n"); j >= 0 {
		s = s[:j]
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil || id == 0 {
		t.Fatalf("output %q does not start with an id after %q", out, marker)
	}
	return uint(id)
}

// listedEntry is the part of `ls --json` the test reads.
type listedEntry struct {
	ID        uint       `json:"id"`
	ProjectID uint       `json:"project_id"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at"`
	Timezone  string     `json:"timezone"`
	Note      string     `json:"note"`
}

func decodeEntries(t *testing.T, out string) []listedEntry {
	t.Helper()
	var entries []listedEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("entries %q are not JSON: %v", out, err)
	}
	return entries
}

func entryWithNote(t *testing.T, entries []listedEntry, note string) listedEntry {
	t.Helper()
	for _, e := range entries {
		if e.Note == note {
			return e
		}
	}
	t.Fatalf("no entry with note %q in %+v", note, entries)
	return listedEntry{}
}

// reportRow is the part of a report row the test reads.
type reportRow struct {
	Bucket string `json:"bucket"`
}

func reportRows(t *testing.T, out string) []reportRow {
	t.Helper()
	var report struct {
		Rows []reportRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report %q is not JSON: %v", out, err)
	}
	return report.Rows
}

// withHome replaces HOME so a run cannot see the config a previous login saved.
func withHome(env []string, home string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, "HOME=") {
			out = append(out, e)
		}
	}
	return append(out, "HOME="+home)
}
