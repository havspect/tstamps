// Package e2e walks the whole product: both binaries, one workspace.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestEndToEnd is the v1 acceptance walk: bootstrap an admin with the server
// binary, serve on a free port, build the team structure over the API, then log,
// time, export and report through the CLI and MCP.
func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	serverBin := build(t, dir, "tstamps-server", "../cmd/tstamps-server")
	cliBin := build(t, dir, "tstamps", "../cmd/tstamps")

	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "TSTAMPS_URL=", "TSTAMPS_TOKEN=", "TSTAMPS_DB=")
	db := filepath.Join(dir, "tstamps.db")
	cli := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(cliBin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tstamps %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	server := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(serverBin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tstamps-server %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	adminPassword := passwordFrom(t, server("workspace", "create", "example.com", "--db", db, "--email", "admin@example.com", "--name", "Ada Admin"))

	base, stopServer := startServer(t, serverBin, db)
	defer stopServer()
	url := base + "/w/example.com"

	cli("login", "--url", url, "--email", "admin@example.com", "--password", adminPassword)
	token := savedToken(t, home)

	// Teams, leads and a member are admin screens in v1; the CLI reads them.
	platform := createTeam(t, url, token, "Platform")
	mobile := createTeam(t, url, token, "Mobile")
	leadA := createUser(t, url, token, "grace@example.com", "Grace Hopper")
	leadB := createUser(t, url, token, "hedy@example.com", "Hedy Lamarr")
	member := createUser(t, url, token, "linus@example.com", "Linus Torvalds")
	setMembers(t, url, token, platform.ID, []memberRole{{leadA.ID, "lead"}, {member.ID, "member"}})
	setMembers(t, url, token, mobile.ID, []memberRole{{leadB.ID, "lead"}})

	cli("projects", "add", "Timeline", "--team", "Platform")

	// Last week, so the range is fixed whatever day the test runs.
	now := time.Now().UTC()
	thisMonday := time.Date(now.Year(), now.Month(), now.Day()-((int(now.Weekday())+6)%7), 0, 0, 0, 0, time.UTC)
	lastMonday := thisMonday.AddDate(0, 0, -7)
	from, to := lastMonday.Format("2006-01-02"), thisMonday.Format("2006-01-02")
	logged := cli("add", "Timeline", "--date", lastMonday.AddDate(0, 0, 1).Format("2006-01-02"),
		"--from", "09:00", "--to", "11:30", "--note", "drag, resize")
	if !strings.Contains(logged, "2h30m") {
		t.Fatalf("add printed %q, want the 2h30m entry", logged)
	}

	started := cli("start", "Timeline", "--note", "MCP")
	if !strings.Contains(started, "started") {
		t.Fatalf("start printed %q", started)
	}
	if stopped := cli("stop", "--note", "done"); !strings.Contains(stopped, "stopped") {
		t.Fatalf("stop printed %q", stopped)
	}

	csv := cli("report", "--from", from, "--to", to, "--by", "week", "--csv")
	if !strings.Contains(csv, "date,iso_week,user,email,team,project,timezone,start,end,seconds,note") {
		t.Fatalf("CSV header missing:\n%s", csv)
	}
	if !strings.Contains(csv, `"drag, resize"`) || !strings.Contains(csv, ",9000,") {
		t.Fatalf("CSV does not carry the logged entry:\n%s", csv)
	}
	report := cli("report", "--from", from, "--to", to, "--by", "week", "--json")
	if !strings.Contains(report, `"total_seconds": 9000`) {
		t.Fatalf("JSON report = %s, want 9000 seconds", report)
	}

	// The MCP client initializes over stdio and reports the same week.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.Command(cliBin, "mcp")
	cmd.Env = env
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "e2e", Version: "1"}, nil).
		Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("mcp connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{
		Name:      "report",
		Arguments: map[string]any{"from": from, "to": to, "by": "week"},
	})
	if err != nil {
		t.Fatalf("mcp report: %v", err)
	}
	if res.IsError {
		t.Fatalf("mcp report failed: %v", res.Content)
	}
	body, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(body), `"total_seconds":9000`) {
		t.Fatalf("mcp report = %s, want 9000 seconds", body)
	}
}

// build compiles one command into dir and returns its path.
func build(t *testing.T, dir, name, pkg string) string {
	t.Helper()
	bin := filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

// passwordFrom extracts the only secret on the single output line.
func passwordFrom(t *testing.T, line string) string {
	t.Helper()
	const marker = "initial password: "
	i := strings.Index(line, marker)
	if i < 0 {
		t.Fatalf("output %q has no %q", line, marker)
	}
	return strings.TrimSpace(line[i+len(marker):])
}

// startServer runs the binary on a free port and returns its base URL once it
// logs the address it actually bound.
func startServer(t *testing.T, bin, db string) (string, func()) {
	t.Helper()
	return startServe(t, bin, "serve", "--db", db, "--addr", "127.0.0.1:0")
}

// startServe runs `tstamps-server` with args on a free port and returns its base
// URL once it logs the address it actually bound.
func startServe(t *testing.T, bin string, args ...string) (string, func()) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	var logs []string
	deadline := time.After(15 * time.Second)
	var url string
	for url == "" {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("server exited before logging its address:\n%s", strings.Join(logs, "\n"))
			}
			logs = append(logs, line)
			for _, field := range strings.Fields(line) {
				if addr, ok := strings.CutPrefix(field, "addr="); ok && strings.Contains(line, "serving") {
					url = "http://" + addr
				}
			}
		case <-deadline:
			t.Fatalf("server did not log its address:\n%s", strings.Join(logs, "\n"))
		}
	}
	go func() {
		for range lines {
		}
	}()

	stop := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	return url, stop
}

// savedToken reads the token the login command stored.
func savedToken(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, ".config", "tstamps", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Token == "" {
		t.Fatal("login stored no token")
	}
	return cfg.Token
}

type memberRole struct {
	UserID uint
	Role   string
}

func createTeam(t *testing.T, url, token, name string) struct{ ID uint } {
	t.Helper()
	var out struct{ ID uint }
	apiCall(t, url, token, http.MethodPost, "/teams", map[string]string{"name": name}, &out)
	return out
}

func createUser(t *testing.T, url, token, email, name string) struct {
	ID              uint
	InitialPassword string `json:"initial_password"`
} {
	t.Helper()
	var out struct {
		ID              uint
		InitialPassword string `json:"initial_password"`
	}
	apiCall(t, url, token, http.MethodPost, "/users", map[string]any{
		"email": email, "name": name, "timezone": "UTC",
	}, &out)
	if out.InitialPassword == "" {
		t.Fatalf("user %s came back without an initial password", email)
	}
	return out
}

func setMembers(t *testing.T, url, token string, teamID uint, members []memberRole) {
	t.Helper()
	body := make([]map[string]any, 0, len(members))
	for _, m := range members {
		body = append(body, map[string]any{"user_id": m.UserID, "role": m.Role})
	}
	apiCall(t, url, token, http.MethodPut, fmt.Sprintf("/teams/%d/members", teamID), body, nil)
}

func apiCall(t *testing.T, url, token, method, path string, body, out any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url+"/api/v1"+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s = %d: %s", method, path, resp.StatusCode, detail)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s decode: %v", method, path, err)
		}
	}
}
