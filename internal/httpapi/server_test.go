package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/audit"
	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/mcpserver"
	"github.com/havspect/tstamps/internal/onboarding"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

func setupTestServer(t *testing.T) (http.Handler, *Dependencies) {
	t.Helper()

	db, err := gormrepo.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := gormrepo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	clk := clock.System{}
	uow := gormrepo.NewTxRunner(db)

	userRepo := gormrepo.NewUserRepository(db)
	projectRepo := gormrepo.NewProjectRepository(db)
	teamRepo := gormrepo.NewTeamRepository(db)
	entryRepo := gormrepo.NewEntryRepository(db)
	auditRepo := gormrepo.NewAuditRepository(db)
	onboardingRepo := gormrepo.NewOnboardingRepository(db)
	reportSource := gormrepo.NewReportSource(db)

	policy := authz.NewPolicyEngine()
	auditSvc := audit.New(auditRepo, clk)
	userSvc := user.New(userRepo, auditSvc, entryRepo, clk)
	teamSvc := team.New(teamRepo, teamRepo, auditSvc, clk)
	projectSvc := project.NewService(projectRepo, uow, auditSvc, projectRepo, policy, clk)
	entrySvc := entry.NewService(entryRepo, uow, auditSvc, entryRepo, policy, clk)
	reportSvc := report.NewService(reportSource, policy, clk)
	onboardingSvc := onboarding.NewService(onboardingRepo)

	mcpHandler := mcpserver.NewHandler(mcpserver.Config{
		UserService:    userSvc,
		ProjectService: projectSvc,
		TeamService:    teamSvc,
		EntryService:   entrySvc,
		ReportService:  reportSvc,
	})

	deps := &Dependencies{
		DB:                db,
		Clock:             clk,
		UserService:       userSvc,
		ProjectService:    projectSvc,
		TeamService:       teamSvc,
		EntryService:      entrySvc,
		ReportService:     reportSvc,
		AuditService:      auditSvc,
		OnboardingService: onboardingSvc,
		MCPHandler:        mcpHandler,
	}

	handler := New(*deps)
	return handler, deps
}

func TestHealthz(t *testing.T) {
	handler, _ := setupTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}

	var body struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}

	if body.Status != "ok" || body.SchemaVersion <= 0 {
		t.Errorf("unexpected health body: %+v", body)
	}
}

func TestOpenAPISpec(t *testing.T) {
	spec, err := Spec()
	if err != nil {
		t.Fatalf("Spec() error: %v", err)
	}
	if len(spec) == 0 {
		t.Fatalf("Spec() returned empty bytes")
	}

	specStr := string(spec)
	for _, expectedPath := range []string{"/api/v1/entries", "/api/v1/projects", "/api/v1/teams", "/api/v1/users"} {
		if !strings.Contains(specStr, expectedPath) {
			t.Errorf("expected spec to contain %s", expectedPath)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler, _ := setupTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("missing Content-Security-Policy header")
	}
	if nosniff := rec.Header().Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", nosniff)
	}
	if ref := rec.Header().Get("Referrer-Policy"); ref != "same-origin" {
		t.Errorf("Referrer-Policy = %q, want same-origin", ref)
	}
}

func TestProtectedEndpointsWithoutAuth(t *testing.T) {
	handler, _ := setupTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	var prob struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != urnUnauthorized {
		t.Errorf("type = %q, want %q", prob.Type, urnUnauthorized)
	}
}

func TestAuthSessionAndPersonalToken(t *testing.T) {
	handler, deps := setupTestServer(t)

	// Create workspace and admin user
	ws := &gormrepo.Workspace{Slug: "acme", CreatedAt: time.Now()}
	if err := deps.DB.Create(ws).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)
	hash, _ := auth.HashPassword("secretpass123")
	u := &gormrepo.User{
		WorkspaceID:        ws.ID,
		Email:              "admin@acme.com",
		Name:               "Admin",
		PasswordHash:       hash,
		Timezone:           "UTC",
		IsAdmin:            true,
		Active:             true,
		MustChangePassword: false,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	if err := deps.DB.WithContext(ctxWs).Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 1. Test Login
	loginBody := `{"email":"admin@acme.com","password":"secretpass123"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookie {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("session cookie not found in login response")
	}

	// 2. Test /api/v1/me with session cookie
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(sessCookie)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var me MeResponse
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if me.Email != "admin@acme.com" || !me.IsAdmin {
		t.Errorf("unexpected me: %+v", me)
	}

	// 3. Test Personal API Token
	rawToken, tokHash, err := auth.NewAPIToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	apiTok := &gormrepo.APIToken{
		WorkspaceID: ws.ID,
		UserID:      u.ID,
		Name:        "cli",
		TokenHash:   tokHash,
		ExpiresAt:   time.Now().AddDate(1, 0, 0),
		CreatedAt:   time.Now(),
	}
	if err := deps.DB.WithContext(ctxWs).Create(apiTok).Error; err != nil {
		t.Fatalf("create api token: %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("projects status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRobotsAndSitemap(t *testing.T) {
	handler, _ := setupTestServer(t)

	// robots
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "User-agent:") {
		t.Errorf("unexpected robots response: %d, %s", rec.Code, rec.Body.String())
	}

	// sitemap
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "urlset") {
		t.Errorf("unexpected sitemap response: %d, %s", rec.Code, rec.Body.String())
	}
}

func TestWebSPA(t *testing.T) {
	handler, _ := setupTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("SPA status = %d, want 200", rec.Code)
	}
}

func TestFullDomainOperations(t *testing.T) {
	handler, deps := setupTestServer(t)

	ws := &gormrepo.Workspace{Slug: "acme", CreatedAt: time.Now()}
	_ = deps.DB.Create(ws)
	ctxWs := gormrepo.WithWorkspaceID(context.Background(), ws.ID)

	hash, _ := auth.HashPassword("adminpass123")
	admin := &gormrepo.User{
		WorkspaceID: ws.ID,
		Email:       "admin@acme.com",
		Name:        "Admin",
		PasswordHash: hash,
		IsAdmin:     true,
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = deps.DB.WithContext(ctxWs).Create(admin)

	rawToken, tokHash, _ := auth.NewAPIToken()
	apiTok := &gormrepo.APIToken{
		WorkspaceID: ws.ID,
		UserID:      admin.ID,
		Name:        "token",
		TokenHash:   tokHash,
		ExpiresAt:   time.Now().AddDate(1, 0, 0),
		CreatedAt:   time.Now(),
	}
	_ = deps.DB.WithContext(ctxWs).Create(apiTok)

	authHeader := "Bearer " + rawToken

	// 1. Teams: Create & List
	createTeamJSON := `{"name":"Backend"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(createTeamJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create team status=%d: %s", rec.Code, rec.Body.String())
	}
	var teamResp teamResponse
	_ = json.NewDecoder(rec.Body).Decode(&teamResp)

	// 2. Projects: Create & List
	createProjJSON := `{"team_id":` + jsonNum(teamResp.ID) + `,"name":"API Service","color":"#00ff00"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(createProjJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d: %s", rec.Code, rec.Body.String())
	}
	var projResp projectResponse
	_ = json.NewDecoder(rec.Body).Decode(&projResp)

	// 3. Timer: Start, GetRunning, Stop
	startTimerJSON := `{"project_id":` + jsonNum(projResp.ID) + `,"note":"Coding"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/timer/start", strings.NewReader(startTimerJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start timer status=%d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/timer", nil)
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get timer status=%d: %s", rec.Code, rec.Body.String())
	}

	stopTimerJSON := `{"note":"Finished"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/timer/stop", strings.NewReader(stopTimerJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop timer status=%d: %s", rec.Code, rec.Body.String())
	}

	// 4. Entries: List
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list entries status=%d: %s", rec.Code, rec.Body.String())
	}

	// 5. Reports: Get & CSV
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil)
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get report status=%d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports.csv", nil)
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get report csv status=%d: %s", rec.Code, rec.Body.String())
	}

	// 6. Users: Create & List
	createUserJSON := `{"email":"dev@acme.com","name":"Developer"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(createUserJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user status=%d: %s", rec.Code, rec.Body.String())
	}

	// 7. Audit: Query
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	req.Header.Set("Authorization", authHeader)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query audit status=%d: %s", rec.Code, rec.Body.String())
	}

	// 8. Onboarding: Validate Slug & Start Signup
	valSlugJSON := `{"slug":"newworkspace"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/signup/validate-slug", strings.NewReader(valSlugJSON))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate slug status=%d: %s", rec.Code, rec.Body.String())
	}
}

func jsonNum(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}

