package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/havspect/tstamps/internal/clock"
)

func TestHostIsolatesWorkspaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workspaces")
	alphaPassword, err := Create(dir, "alpha", "ada@example.com", "Ada Alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	betaPassword, err := Create(dir, "beta", "ada@example.com", "Ada Beta")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	host, err := New(dir, clock.System{}, nil)
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
	dir := filepath.Join(t.TempDir(), "workspaces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	host, err := New(dir, clock.System{}, nil)
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("requests created files in the workspace directory: %v", entries)
	}

	// The process health check inspects the directory, not every workspace.
	if got := hostedGet(t, server.URL+"/healthz", "", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"ok"`) {
		t.Fatalf("hosted health = %d %s", got.Code, got.Body)
	}
}

func TestHostConcurrentFirstOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workspaces")
	if _, err := Create(dir, "alpha", "ada@example.com", "Ada"); err != nil {
		t.Fatalf("create: %v", err)
	}
	host, err := New(dir, clock.System{}, nil)
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
		t.Fatalf("cache has %d entries, want one store for concurrent requests", cached)
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
	dir := t.TempDir()
	host, err := New(dir, clock.System{}, nil)
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
	configured, err := New(dir, clock.System{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
