package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/store"
)

func TestOAuthDiscoveryAndPKCEFlow(t *testing.T) {
	reg, err := store.OpenRegistry(filepath.Join(t.TempDir(), "tstamps.db"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ws := &store.Workspace{Slug: "alpha"}
	if err := reg.CreateWorkspace(ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wst := reg.ForWorkspace(ws.ID)
	clk := clock.System{}
	srv := New(wst, clk)

	// 1. Discovery
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("discovery status %d", rec.Code)
	}
	var disc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &disc); err != nil {
		t.Fatalf("unmarshal discovery: %v", err)
	}
	if disc["issuer"] == "" {
		t.Fatalf("expected issuer")
	}

	// 2. Protected Resource
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("protected resource status %d", rec.Code)
	}

	// 3. Dynamic Client Registration
	regBody, _ := json.Marshal(map[string]any{
		"client_name":   "Test Client",
		"redirect_uris": []string{"http://127.0.0.1/callback"},
	})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, apiPrefix+"/oauth/register", strings.NewReader(string(regBody)))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status %d: %s", rec.Code, rec.Body.String())
	}
	var regResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &regResp)
	clientID, _ := regResp["client_id"].(string)
	if clientID == "" {
		t.Fatalf("expected client_id")
	}

	// Create user & session for authorize
	u := &store.User{Email: "user@example.com", Name: "User", PasswordHash: "hash", Timezone: "UTC", Active: true}
	if err := wst.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessToken, sessHash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatalf("new session token: %v", err)
	}
	if err := wst.CreateSession(&store.Session{UserID: u.ID, TokenHash: sessHash, ExpiresAt: clk.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// PKCE S256 verifier & challenge
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk-extra-chars-for-43-length"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authQuery := "?response_type=code&client_id=" + clientID +
		"&redirect_uri=" + url.QueryEscape("http://127.0.0.1/callback") +
		"&scope=tstamps.read&state=xyz12345" +
		"&code_challenge=" + challenge +
		"&code_challenge_method=S256"

	// 4. Authorize GET
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, apiPrefix+"/oauth/authorize"+authQuery, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessToken})
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorize get status %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Authorize POST (Approve)
	approveBody, _ := json.Marshal(map[string]any{"approve": true})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "http://example.com"+apiPrefix+"/oauth/authorize"+authQuery, strings.NewReader(string(approveBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessToken})
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorize post status %d: %s", rec.Code, rec.Body.String())
	}
	var authResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &authResp)
	redirectTo := authResp["redirect_to"]
	uURL, _ := url.Parse(redirectTo)
	code := uURL.Query().Get("code")
	if code == "" {
		t.Fatalf("expected authorization code, got %s", redirectTo)
	}

	// 6. Token Exchange (Authorization Code + PKCE verifier)
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("redirect_uri", "http://127.0.0.1/callback")
	form.Set("code_verifier", verifier)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, apiPrefix+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token exchange status %d: %s", rec.Code, rec.Body.String())
	}
	var tokenResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tokenResp)
	accessToken, _ := tokenResp["access_token"].(string)
	refreshToken, _ := tokenResp["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("expected access_token and refresh_token, got %+v", tokenResp)
	}

	// 7. Test Scope Enforcement (Read allowed, write forbidden)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, apiPrefix+"/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /me with read token status %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, apiPrefix+"/entries", strings.NewReader(`{"project_id":1,"start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z","timezone":"UTC"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /entries with read-only token expected 403, got %d", rec.Code)
	}

	// 8. Refresh Token Rotation
	form = url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, apiPrefix+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh token status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceGrantAndRemoteMCP(t *testing.T) {
	reg, err := store.OpenRegistry(filepath.Join(t.TempDir(), "tstamps.db"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer reg.Close()
	if err := reg.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ws := &store.Workspace{Slug: "beta"}
	if err := reg.CreateWorkspace(ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wst := reg.ForWorkspace(ws.ID)
	clk := clock.System{}
	srv := New(wst, clk)

	u := &store.User{Email: "cli@example.com", Name: "CLI User", PasswordHash: "hash", Timezone: "UTC", Active: true}
	if err := wst.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessToken, sessHash, _ := auth.NewSessionToken()
	_ = wst.CreateSession(&store.Session{UserID: u.ID, TokenHash: sessHash, ExpiresAt: clk.Now().Add(time.Hour)})

	// 1. Start device auth
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/device", strings.NewReader(`{"client_id":"tstamps-cli"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start device status %d: %s", rec.Code, rec.Body.String())
	}
	var devResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &devResp)
	deviceCode, _ := devResp["device_code"].(string)
	userCode, _ := devResp["user_code"].(string)

	// 2. Poll before approval -> authorization_pending
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/device/token", strings.NewReader(`{"device_code":"`+deviceCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "authorization_pending") {
		t.Fatalf("expected authorization_pending, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Approve device code
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "http://example.com"+apiPrefix+"/auth/device/approve", strings.NewReader(`{"user_code":"`+userCode+`","approve":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessToken})
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve device status %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Poll after approval -> 200 with tokens
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, apiPrefix+"/auth/device/token", strings.NewReader(`{"device_code":"`+deviceCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("device token status %d: %s", rec.Code, rec.Body.String())
	}
	var tokResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tokResp)
	accessToken, _ := tokResp["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("expected access_token from device flow")
	}

	// 5. Unauthenticated /mcp returns 401 with WWW-Authenticate
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), "oauth-protected-resource") {
		t.Fatalf("expected 401 with WWW-Authenticate on /mcp, got %d (%s)", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	// 6. Authenticated /mcp with bearer token does NOT return 401 or 403
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("expected authenticated MCP access, got %d: %s", rec.Code, rec.Body.String())
	}
}
