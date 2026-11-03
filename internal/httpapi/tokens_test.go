package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

// setTokenScopes writes the scopes column directly, past tenancy, to stand in for a stored token.
func setTokenScopes(t *testing.T, deps *Dependencies, userID uint, scopes string) {
	t.Helper()

	ctx := gormrepo.WithBypassTenancy(context.Background())
	if err := deps.DB.WithContext(ctx).Model(&gormrepo.APIToken{}).
		Where("user_id = ?", userID).Update("scopes", scopes).Error; err != nil {
		t.Fatalf("set token scopes: %v", err)
	}
}

func TestPersonalTokenScopesGateRequests(t *testing.T) {
	handler, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "acme"}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	admin, adminAuth := seedToken(t, deps, ws.ID, "admin@acme.com", true)

	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	team := &gormrepo.Team{WorkspaceID: ws.ID, Name: "Platform"}
	if err := deps.DB.WithContext(ctxWs).Create(team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	projectBody := `{"team_id":` + jsonNum(team.ID) + `,"name":"API"}`

	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", adminAuth)
		handler.ServeHTTP(rec, req)
		return rec
	}

	setTokenScopes(t, deps, admin.ID, `["tstamps.read"]`)
	if rec := call(http.MethodGet, "/api/v1/projects", ""); rec.Code != http.StatusOK {
		t.Errorf("read-only token GET status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec := call(http.MethodPost, "/api/v1/projects", projectBody)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), urnForbidden) {
		t.Errorf("read-only token POST status = %d, want 403 %s: %s", rec.Code, urnForbidden, rec.Body.String())
	}

	// Legacy rows carry no scopes and keep full read+write access.
	setTokenScopes(t, deps, admin.ID, "")
	if rec := call(http.MethodPost, "/api/v1/projects", projectBody); rec.Code != http.StatusCreated {
		t.Errorf("legacy token POST status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

func TestDelegatedTokenIsBoundToOAuthClient(t *testing.T) {
	handler, deps, adminAuth := setupDeviceTestServer(t)
	tok := deviceOAuthToken(t, handler, adminAuth)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/tokens", strings.NewReader(`{"name":"delegated"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var row gormrepo.APIToken
	ctx := gormrepo.WithBypassTenancy(context.Background())
	if err := deps.DB.WithContext(ctx).Where("name = ?", "delegated").First(&row).Error; err != nil {
		t.Fatalf("read token row: %v", err)
	}
	if row.ClientID != "tstamps-cli" {
		t.Errorf("client_id = %q, want tstamps-cli", row.ClientID)
	}
	var scopes []string
	if err := json.Unmarshal([]byte(row.Scopes), &scopes); err != nil {
		t.Fatalf("decode scopes %q: %v", row.Scopes, err)
	}
	if len(scopes) != 2 || scopes[0] != "tstamps.read" || scopes[1] != "tstamps.write" {
		t.Errorf("scopes = %v, want [tstamps.read tstamps.write]", scopes)
	}
}
