package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

func TestHostIsolatesWorkspaces(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	alphaPassword, err := Create(db, "alpha", "ada@example.com", "Ada Alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	betaPassword, err := Create(db, "beta", "ada@example.com", "Ada Beta")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	alphaCookie, alphaToken := loginHosted(t, server.URL, "alpha", "ada@example.com", alphaPassword)
	betaCookie, _ := loginHosted(t, server.URL, "beta", "ada@example.com", betaPassword)

	// The same email is a different account, and a session or token from one
	// database cannot authenticate against the other.
	if got := hostedGet(t, server.URL+"/w/alpha/api/v1/me", alphaCookie, ""); got.Code != http.StatusOK {
		t.Fatalf("alpha me = %d, want 200: %s", got.Code, got.Body)
	}
	if got := hostedGet(t, server.URL+"/w/beta/api/v1/me", alphaCookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("alpha cookie on beta = %d, want 401", got.Code)
	}
	if got := hostedGet(t, server.URL+"/w/beta/api/v1/me", betaCookie, ""); got.Code != http.StatusOK {
		t.Fatalf("beta me = %d, want 200", got.Code)
	}
	if got := hostedGet(t, server.URL+"/w/beta/api/v1/me", "", alphaToken); got.Code != http.StatusUnauthorized {
		t.Fatalf("alpha token on beta = %d, want 401", got.Code)
	}
	if got := hostedGet(t, server.URL+"/w/alpha/api/v1/me", "", alphaToken); got.Code != http.StatusOK {
		t.Fatalf("alpha token on alpha = %d, want 200", got.Code)
	}

	// Unprefixed API calls have no workspace and must never guess one.
	got := hostedGet(t, server.URL+"/api/v1/me", "", "")
	if got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "urn:tstamps:error:not-found") {
		t.Fatalf("unprefixed API = %d %s, want problem 404", got.Code, got.Body)
	}
}

func TestHostUnknownWorkspaceNeverCreatesOne(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	for _, path := range []string{"/w/missing/api/v1/me", "/w/Bad/api/v1/me", "/w/../api/v1/me"} {
		if got := hostedGet(t, server.URL+path, "", ""); got.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, got.Code)
		}
	}
	// The shared database exists, but no request may add a workspace to it.
	reg, err := store.OpenRegistry(db)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	workspaces, err := reg.Workspaces()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	_ = reg.Close()
	if len(workspaces) != 0 {
		t.Fatalf("requests created %d workspaces: %v", len(workspaces), workspaces)
	}

	// The process health check pings the database, not every workspace.
	if got := hostedGet(t, server.URL+"/healthz", "", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"ok"`) {
		t.Fatalf("hosted health = %d %s", got.Code, got.Body)
	}
}

func TestHostConcurrentFirstOpen(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	if _, err := Create(db, "alpha", "ada@example.com", "Ada"); err != nil {
		t.Fatalf("create: %v", err)
	}
	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(server.URL + "/w/alpha/healthz")
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	host.mu.Lock()
	cached := len(host.cache)
	host.mu.Unlock()
	if cached != 1 {
		t.Fatalf("cache has %d entries, want one handler for concurrent requests", cached)
	}
}

func TestHostKeepsRowsOfEachWorkspaceApart(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	alphaPassword, err := Create(db, "alpha", "ada@example.com", "Ada Alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	betaPassword, err := Create(db, "beta", "ada@example.com", "Ada Beta")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	reg, err := store.OpenRegistry(db)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	alphaStore := storeFor(t, reg, "alpha")
	betaStore := storeFor(t, reg, "beta")
	alphaAdmin, err := alphaStore.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("alpha admin: %v", err)
	}
	betaAdmin, err := betaStore.UserByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("beta admin: %v", err)
	}
	alphaProject, alphaEntry, alphaTeam := seedWorkspace(t, alphaStore, alphaAdmin, "Alpha")
	_, _, _ = seedWorkspace(t, betaStore, betaAdmin, "Beta")
	_ = reg.Close()

	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})
	alphaCookie, _ := loginHosted(t, server.URL, "alpha", "ada@example.com", alphaPassword)
	betaCookie, _ := loginHosted(t, server.URL, "beta", "ada@example.com", betaPassword)

	var projects []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	decodeJSON(t, hostedGet(t, server.URL+"/w/alpha/api/v1/projects", alphaCookie, ""), &projects)
	if len(projects) != 1 || projects[0].ID != alphaProject.ID || projects[0].Name != "Project Alpha" {
		t.Fatalf("alpha projects = %+v, want only its own", projects)
	}
	var users []struct {
		ID    uint   `json:"id"`
		Email string `json:"email"`
	}
	decodeJSON(t, hostedGet(t, server.URL+"/w/beta/api/v1/users", betaCookie, ""), &users)
	if len(users) != 1 || users[0].ID != betaAdmin.ID {
		t.Fatalf("beta users = %+v, want only its own", users)
	}

	// An ID from the other workspace is not found, not merely forbidden.
	cross := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPatch, "/w/beta/api/v1/entries/" + fmt.Sprint(alphaEntry.ID), map[string]string{"note": "cross-tenant write"}},
		{http.MethodDelete, "/w/beta/api/v1/entries/" + fmt.Sprint(alphaEntry.ID), map[string]any{}},
		{http.MethodPost, "/w/beta/api/v1/projects/" + fmt.Sprint(alphaProject.ID) + "/restore", map[string]any{}},
		{http.MethodPatch, "/w/beta/api/v1/users/" + fmt.Sprint(alphaAdmin.ID), map[string]string{"name": "cross-tenant write"}},
		{http.MethodPost, "/w/beta/api/v1/users/" + fmt.Sprint(alphaAdmin.ID) + "/password", map[string]any{}},
		{http.MethodDelete, "/w/beta/api/v1/teams/" + fmt.Sprint(alphaTeam.ID), map[string]any{}},
		{http.MethodPut, "/w/beta/api/v1/teams/" + fmt.Sprint(alphaTeam.ID) + "/members", []any{}},
	}
	for _, tc := range cross {
		got := hostedJSON(t, tc.method, server.URL+tc.path, tc.body, betaCookie, "")
		if got.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(got.Body)
			t.Errorf("%s %s = %d: %s, want 404", tc.method, tc.path, got.StatusCode, body)
		}
		_ = got.Body.Close()
	}

	// Filters cannot pull another workspace's rows either.
	var audit struct {
		Events []struct {
			TargetID uint `json:"target_id"`
		} `json:"events"`
	}
	decodeJSON(t, hostedGet(t, server.URL+"/w/beta/api/v1/audit?target_type=project&target_id="+fmt.Sprint(alphaProject.ID), betaCookie, ""), &audit)
	if len(audit.Events) != 0 {
		t.Fatalf("beta audit by alpha project = %+v, want none", audit.Events)
	}
	var report struct {
		Rows []struct {
			ProjectID uint `json:"project_id"`
		} `json:"rows"`
	}
	decodeJSON(t, hostedGet(t, server.URL+"/w/beta/api/v1/reports?from=2026-10-19&to=2026-10-26&by=none&project_id="+fmt.Sprint(alphaProject.ID), betaCookie, ""), &report)
	if len(report.Rows) != 0 {
		t.Fatalf("beta report by alpha project = %+v, want no rows", report.Rows)
	}
}

func TestHostPicksUpWorkspaceCreatedAfterStart(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	if got := hostedGet(t, server.URL+"/w/late/healthz", "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace health = %d, want 404", got.Code)
	}
	if _, err := Create(db, "late", "ada@example.com", "Ada"); err != nil {
		t.Fatalf("create after start: %v", err)
	}
	if got := hostedGet(t, server.URL+"/w/late/healthz", "", ""); got.Code != http.StatusOK {
		t.Fatalf("new workspace health = %d, want 200: %s", got.Code, got.Body)
	}
}

func TestHostLoginSkipsThePromptWhenThereIsOneWorkspace(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	if _, err := Create(db, "only", "ada@example.com", "Ada"); err != nil {
		t.Fatalf("create: %v", err)
	}
	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/w/only/login" {
		t.Fatalf("/login = %d %q, want 302 /w/only/login", resp.StatusCode, resp.Header.Get("Location"))
	}
	var only struct {
		Slug string `json:"slug"`
	}
	decodeJSON(t, hostedGet(t, server.URL+"/api/v1/workspace", "", ""), &only)
	if only.Slug != "only" {
		t.Fatalf("only workspace = %q, want only", only.Slug)
	}

	// A second workspace restores the prompt instead of guessing one.
	if _, err := Create(db, "second", "grace@example.com", "Grace"); err != nil {
		t.Fatalf("create second: %v", err)
	}
	resp, err = client.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Location") != "" {
		t.Fatalf("/login with two workspaces = %d %q, want the prompt", resp.StatusCode, resp.Header.Get("Location"))
	}
	decodeJSON(t, hostedGet(t, server.URL+"/api/v1/workspace", "", ""), &only)
	if only.Slug != "" {
		t.Fatalf("two workspaces returned slug %q, want none", only.Slug)
	}
}

func storeFor(t *testing.T, reg *store.Registry, slug string) *store.Store {
	t.Helper()
	w, err := reg.WorkspaceBySlug(slug)
	if err != nil {
		t.Fatalf("workspace %s: %v", slug, err)
	}
	return reg.ForWorkspace(w.ID)
}

func seedWorkspace(t *testing.T, st *store.Store, admin *store.User, label string) (*store.Project, *store.Entry, *store.Team) {
	t.Helper()
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	team := &store.Team{Name: "Team " + label}
	if err := st.CreateTeam(team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	project := &store.Project{TeamID: team.ID, Name: "Project " + label}
	if err := st.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	end := now.Add(time.Hour)
	entry := &store.Entry{UserID: admin.ID, ProjectID: project.ID, StartAt: now, EndAt: &end, Timezone: "UTC", Note: label}
	if err := st.CreateEntry(entry); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	return project, entry, team
}

func decodeJSON(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.Code, resp.Body)
	}
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", resp.Body, err)
	}
}

func loginHosted(t *testing.T, base, slug, email, password string) (cookie, token string) {
	t.Helper()
	resp := hostedJSON(t, http.MethodPost, base+"/w/"+slug+"/api/v1/auth/login",
		map[string]string{"email": email, "password": password}, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login %s = %d: %s", slug, resp.StatusCode, body)
	}
	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "session") {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatalf("login %s set no session cookie", slug)
	}

	resp = hostedJSON(t, http.MethodPost, base+"/w/"+slug+"/api/v1/me/tokens",
		map[string]string{"name": "test"}, cookie, "")
	defer resp.Body.Close()
	var created struct {
		Token string `json:"token"`
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create token %s = %d: %s", slug, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	return cookie, created.Token
}

func hostedGet(t *testing.T, url, cookie, token string) *httptest.ResponseRecorder {
	t.Helper()
	resp := hostedJSON(t, http.MethodGet, url, nil, cookie, token)
	defer resp.Body.Close()
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	for k, v := range resp.Header {
		rec.Header()[k] = v
	}
	_, _ = io.Copy(rec.Body, resp.Body)
	return rec
}

func hostedJSON(t *testing.T, method, url string, body any, cookie, token string) *http.Response {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+strings.Split(strings.TrimPrefix(url, "http://"), "/")[0])
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "tstamps_session", Value: cookie})
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHostAnswersSignupBeforeAnyWorkspaceExists(t *testing.T) {
	db := filepath.Join(t.TempDir(), "tstamps.db")
	host, err := New(db, clock.System{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(host)
	t.Cleanup(func() {
		server.Close()
		_ = host.Close()
	})

	// With no email configured the route still exists and explains itself.
	resp, err := http.Post(server.URL+"/api/v1/signup", "application/json", strings.NewReader(`{"email":"ada@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "urn:tstamps:error:unavailable") {
		t.Fatalf("signup = %d %s, want 503 problem", resp.StatusCode, body)
	}

	// A configured handler sees the request at the root, not inside a workspace.
	reached := false
	configured, err := New(db, clock.System{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if r.URL.Path != "/api/v1/signup/verify" {
			t.Errorf("path = %q, want the unprefixed signup path", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer configured.Close()
	rec := httptest.NewRecorder()
	configured.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/signup/verify?token=x", nil))
	if !reached || rec.Code != http.StatusNoContent {
		t.Fatalf("configured signup not reached: reached=%v status=%d", reached, rec.Code)
	}
}
