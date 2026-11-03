package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/oauth"
)

// mintDelegatedToken creates a personal token through the OAuth access token's principal.
func mintDelegatedToken(t *testing.T, handler http.Handler, oauthAccessToken string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/tokens", strings.NewReader(`{"name":"delegated"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+oauthAccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create delegated token status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode created token: %v", err)
	}
	return out.Token
}

func TestListGrantsReturnsAnArrayWhenEmpty(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/oauth/grants", nil)
	req.Header.Set("Authorization", adminAuth)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list grants status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("empty grants body = %q, want []", got)
	}
}

func TestRevokeGrantCascadesToMintedTokens(t *testing.T) {
	handler, _, adminAuth := setupDeviceTestServer(t)
	tok := deviceOAuthToken(t, handler, adminAuth)
	minted := mintDelegatedToken(t, handler, tok.AccessToken)

	listGrants := func() []oauth.Grant {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/oauth/grants", nil)
		req.Header.Set("Authorization", adminAuth)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list grants status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var grants []oauth.Grant
		if err := json.Unmarshal(rec.Body.Bytes(), &grants); err != nil {
			t.Fatalf("decode grants: %v", err)
		}
		return grants
	}

	findCLI := func(grants []oauth.Grant) *oauth.Grant {
		for i := range grants {
			if grants[i].ClientID == "tstamps-cli" {
				return &grants[i]
			}
		}
		t.Fatalf("no tstamps-cli grant in %+v", grants)
		return nil
	}

	grant := findCLI(listGrants())
	if grant.RequestID == "" {
		t.Errorf("grant id is empty")
	}
	if grant.ClientName != "Tstamps CLI" {
		t.Errorf("client_name = %q, want Tstamps CLI", grant.ClientName)
	}
	if len(grant.Scopes) != 2 || grant.Scopes[0] != "tstamps.read" || grant.Scopes[1] != "tstamps.write" {
		t.Errorf("scopes = %v, want both scopes", grant.Scopes)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/oauth/grants/"+grant.RequestID, nil)
	req.Header.Set("Authorization", adminAuth)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke grant status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	for name, bearer := range map[string]string{
		"oauth access token": tok.AccessToken,
		"minted token":       minted,
	} {
		meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		meReq.Header.Set("Authorization", "Bearer "+bearer)
		meRec := httptest.NewRecorder()
		handler.ServeHTTP(meRec, meReq)
		if meRec.Code != http.StatusUnauthorized {
			t.Errorf("%s after grant revoke status = %d, want 401: %s", name, meRec.Code, meRec.Body.String())
		}
	}

	if got := findCLI(listGrants()); got.RevokedAt == nil {
		t.Errorf("grant revoked_at is nil after revoke")
	}
}
