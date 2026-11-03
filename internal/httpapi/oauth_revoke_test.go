package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// deviceOAuthToken runs the device flow and returns the issued token pair.
func deviceOAuthToken(t *testing.T, handler http.Handler, adminAuth string) deviceTokenResponse {
	t.Helper()

	start := startDevice(t, handler)
	verifyDeviceCode(t, handler, adminAuth, start.UserCode, true)

	rec := pollDeviceToken(t, handler, start.DeviceCode)
	if rec.Code != http.StatusOK {
		t.Fatalf("token poll status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var tok deviceTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	return tok
}

func revokeToken(t *testing.T, handler http.Handler, token, hint string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{
		"token":           {token},
		"token_type_hint": {hint},
		"client_id":       {"tstamps-cli"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRevokeAccessTokenStopsBearerAuth(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)
	tok := deviceOAuthToken(t, handler, adminAuth)

	rec := revokeToken(t, handler, tok.AccessToken, "access_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/me after revoke status = %d, want 401: %s", meRec.Code, meRec.Body.String())
	}
}

func TestRevokeRefreshTokenStopsRefreshGrant(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)
	tok := deviceOAuthToken(t, handler, adminAuth)

	rec := revokeToken(t, handler, tok.RefreshToken, "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {"tstamps-cli"},
	}
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusBadRequest {
		t.Fatalf("refresh after revoke status = %d, want 400: %s", refreshRec.Code, refreshRec.Body.String())
	}
}
