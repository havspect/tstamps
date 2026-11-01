package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/oauth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

type deviceStartResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

func setupDeviceTestServer(t *testing.T) (http.Handler, *Dependencies, string) {
	t.Helper()

	handler, deps := setupTestServer(t)
	deps.OAuthProvider = oauth.NewProvider(gormrepo.NewOAuthStore(deps.DB, 0), nil)
	handler = New(*deps)

	ws := &gormrepo.Workspace{Slug: "acme"}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	_, adminAuth := seedToken(t, deps, ws.ID, "admin@acme.com", true)
	return handler, deps, adminAuth
}

func startDevice(t *testing.T, handler http.Handler) deviceStartResponse {
	t.Helper()

	form := url.Values{
		"client_id": {"tstamps-cli"},
		"scope":     {"tstamps.read tstamps.write"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("device start status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var out deviceStartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode device start: %v", err)
	}
	return out
}

func pollDeviceToken(t *testing.T, handler http.Handler, deviceCode string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
		"client_id":   {"tstamps-cli"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func verifyDeviceCode(t *testing.T, handler http.Handler, bearer, userCode string, approve bool) {
	t.Helper()

	body, _ := json.Marshal(map[string]any{"user_code": userCode, "approve": approve})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("device verify status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceStartReturnsWorkspaceVerificationURI(t *testing.T) {
	handler, _, _ := setupDeviceTestServer(t)
	out := startDevice(t, handler)

	if out.UserCode == "" {
		t.Fatal("device start returned an empty user_code")
	}
	if !strings.HasSuffix(out.VerificationURI, "/w/acme/auth/device") {
		t.Errorf("verification_uri = %q, want suffix /w/acme/auth/device", out.VerificationURI)
	}
	if !strings.HasPrefix(out.VerificationURIComplete, out.VerificationURI+"?user_code=") {
		t.Errorf("verification_uri_complete = %q, want %q plus user_code query", out.VerificationURIComplete, out.VerificationURI)
	}
}

func TestDeviceStartRequiresSingleWorkspace(t *testing.T) {
	handler, deps, _ := setupDeviceTestServer(t)
	if err := deps.DB.Create(&gormrepo.Workspace{Slug: "other"}).Error; err != nil {
		t.Fatalf("create second workspace: %v", err)
	}

	form := url.Values{"client_id": {"tstamps-cli"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("device start status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceFlowHappyPath(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)
	start := startDevice(t, handler)

	pending := pollDeviceToken(t, handler, start.DeviceCode)
	if pending.Code != http.StatusBadRequest {
		t.Fatalf("pending poll status = %d, want 400", pending.Code)
	}
	var pendingBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(pending.Body.Bytes(), &pendingBody)
	if pendingBody.Error != "authorization_pending" {
		t.Errorf("pending poll error = %q, want authorization_pending", pendingBody.Error)
	}

	verifyDeviceCode(t, handler, adminAuth, start.UserCode, true)

	rec := pollDeviceToken(t, handler, start.DeviceCode)
	if rec.Code != http.StatusOK {
		t.Fatalf("token poll status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" || tok.TokenType != "Bearer" {
		t.Fatalf("unexpected token response: %+v", tok)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/me with device token status = %d, want 200: %s", meRec.Code, meRec.Body.String())
	}

	// Refresh the device-issued token without a workspace in the request context.
	refreshForm := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {"tstamps-cli"},
	}
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(refreshForm.Encode()))
	refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200: %s", refreshRec.Code, refreshRec.Body.String())
	}
	var refreshed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(refreshRec.Body.Bytes(), &refreshed); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Fatal("refresh response has no access_token")
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)
	start := startDevice(t, handler)

	verifyDeviceCode(t, handler, adminAuth, start.UserCode, false)

	rec := pollDeviceToken(t, handler, start.DeviceCode)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("denied poll status = %d, want 400", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error != "access_denied" {
		t.Errorf("denied poll error = %q, want access_denied", body.Error)
	}
}

func TestDeviceTokenUnknownCode(t *testing.T) {
	handler, _, _ := setupDeviceTestServer(t)

	rec := pollDeviceToken(t, handler, "dc_does_not_exist")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown code status = %d, want 400", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error != "invalid_grant" {
		t.Errorf("unknown code error = %q, want invalid_grant", body.Error)
	}
}
