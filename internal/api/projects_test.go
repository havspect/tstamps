package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/havspect/tstamps/internal/store"
)

func projectPath(id uint) string {
	return fmt.Sprintf("/api/v1/projects/%d", id)
}

// projectBody is the create body for team with name.
func projectBody(teamID uint, name string) map[string]any {
	return map[string]any{"team_id": teamID, "name": name}
}

// projectOrigin satisfies the same-origin rule for cookie mutations.
func projectOrigin() map[string]string {
	return map[string]string{"Origin": "http://example.com"}
}

func TestProjectList(t *testing.T) {
	f := seed(t)

	if rec := f.do(http.MethodGet, "/api/v1/projects", nil, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d, want 401", rec.Code)
	}

	rec := f.do(http.MethodPatch, projectPath(f.ProjectB.ID), map[string]any{"archived": true}, projectOrigin(), f.as(t, f.Admin))
	if rec.Code != http.StatusOK {
		t.Fatalf("archive = %d: %s", rec.Code, rec.Body)
	}

	// Everyone sees the same active projects, ordered by name.
	for _, tt := range []struct {
		name string
		user *store.User
	}{
		{"member", f.Member},
		{"lead", f.LeadA},
		{"outsider", f.Outsider},
		{"admin", f.Admin},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(http.MethodGet, "/api/v1/projects", nil, nil, f.as(t, tt.user))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			list := decode[[]projectResponse](t, rec)
			if len(list) != 1 || list[0].Name != "Timeline" || list[0].TeamID != f.TeamA.ID || list[0].ArchivedAt != nil {
				t.Fatalf("list = %+v, want only the active Timeline", list)
			}
		})
	}

	for _, u := range []*store.User{f.Member, f.LeadA} {
		if rec := f.do(http.MethodGet, "/api/v1/projects?all=true", nil, nil, f.as(t, u)); rec.Code != http.StatusForbidden {
			t.Fatalf("all=true as %s = %d, want 403", u.Email, rec.Code)
		}
	}

	rec = f.do(http.MethodGet, "/api/v1/projects?all=true", nil, nil, f.as(t, f.Admin))
	if rec.Code != http.StatusOK {
		t.Fatalf("all=true as admin = %d: %s", rec.Code, rec.Body)
	}
	all := decode[[]projectResponse](t, rec)
	if len(all) != 2 || all[0].Name != "Sync" || all[0].ArchivedAt == nil || all[1].Name != "Timeline" {
		t.Fatalf("all = %+v, want Sync archived then Timeline", all)
	}
}

func TestProjectCreate(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()

	if rec := f.do(http.MethodPost, "/api/v1/projects", projectBody(f.TeamA.ID, "Nope"), origin, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d, want 401", rec.Code)
	}

	for _, tt := range []struct {
		name string
		user *store.User
	}{
		{"member", f.Member},
		{"outsider", f.Outsider},
		{"lead of another team", f.LeadB},
	} {
		if rec := f.do(http.MethodPost, "/api/v1/projects", projectBody(f.TeamA.ID, "Nope"), origin, f.as(t, tt.user)); rec.Code != http.StatusForbidden {
			t.Errorf("create as %s = %d, want 403", tt.name, rec.Code)
		}
	}

	rec := f.do(http.MethodPost, "/api/v1/projects", projectBody(f.TeamA.ID, "Payments"), origin, f.as(t, f.LeadA))
	if rec.Code != http.StatusCreated {
		t.Fatalf("lead create = %d, want 201: %s", rec.Code, rec.Body)
	}
	p := decode[projectResponse](t, rec)
	if p.ID == 0 || p.Name != "Payments" || p.TeamID != f.TeamA.ID || p.ArchivedAt != nil {
		t.Fatalf("created = %+v", p)
	}

	rec = f.do(http.MethodPost, "/api/v1/projects", projectBody(f.TeamB.ID, "Billing"), origin, f.as(t, f.Admin))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user = %d, want 201: %s", rec.Code, rec.Body)
	}
	if p := decode[projectResponse](t, rec); p.TeamID != f.TeamB.ID {
		t.Fatalf("admin created in team %d, want %d", p.TeamID, f.TeamB.ID)
	}
}

func TestProjectCreateRejectsUnusableTeam(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()
	at := f.clock.now
	archived := &store.Team{Name: "Old", ArchivedAt: &at}
	if err := f.st.CreateTeam(archived); err != nil {
		t.Fatalf("create archived team: %v", err)
	}

	for _, tt := range []struct {
		name string
		user *store.User
		team uint
	}{
		{"archived team as admin", f.Admin, archived.ID},
		{"archived team as its former lead", f.LeadB, archived.ID},
		{"unknown team as admin", f.Admin, 9999},
	} {
		rec := f.do(http.MethodPost, "/api/v1/projects", projectBody(tt.team, "Nope"), origin, f.as(t, tt.user))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422: %s", tt.name, rec.Code, rec.Body)
			continue
		}
		if body := decode[struct {
			Type string `json:"type"`
		}](t, rec); body.Type != urnInvalidInput {
			t.Errorf("%s type = %q, want %q", tt.name, body.Type, urnInvalidInput)
		}
	}
}

func TestProjectPatch(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()

	if rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"name": "Hijacked"}, origin, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous patch = %d, want 401", rec.Code)
	}

	for _, tt := range []struct {
		name string
		user *store.User
	}{
		{"member", f.Member},
		{"outsider", f.Outsider},
		{"lead of another team", f.LeadB},
	} {
		if rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"name": "Hijacked"}, origin, f.as(t, tt.user)); rec.Code != http.StatusForbidden {
			t.Errorf("patch as %s = %d, want 403", tt.name, rec.Code)
		}
	}

	rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"name": "Timeline v2", "color": "#ff0000"}, origin, f.as(t, f.LeadA))
	if rec.Code != http.StatusOK {
		t.Fatalf("lead patch = %d, want 200: %s", rec.Code, rec.Body)
	}
	if p := decode[projectResponse](t, rec); p.Name != "Timeline v2" || p.Color != "#ff0000" {
		t.Fatalf("patched = %+v", p)
	}
	got, err := f.st.ProjectByID(f.ProjectA.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Name != "Timeline v2" || got.Color != "#ff0000" || got.TeamID != f.TeamA.ID {
		t.Fatalf("stored = %+v", got)
	}
}

func TestProjectMoveTeam(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()

	rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"team_id": f.TeamB.ID}, origin, f.as(t, f.LeadA))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("lead of the current team only = %d, want 403: %s", rec.Code, rec.Body)
	}
	if got, err := f.st.ProjectByID(f.ProjectA.ID); err != nil || got.TeamID != f.TeamA.ID {
		t.Fatalf("project moved to %d despite 403: %+v, %v", f.TeamB.ID, got, err)
	}

	rec = f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"team_id": f.TeamB.ID}, origin, f.as(t, f.Admin))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin move = %d, want 200: %s", rec.Code, rec.Body)
	}
	if p := decode[projectResponse](t, rec); p.TeamID != f.TeamB.ID {
		t.Fatalf("team_id = %d, want %d", p.TeamID, f.TeamB.ID)
	}

	// A lead of both teams may move it too.
	g := seed(t)
	g.membership(t, g.TeamB, g.LeadA, store.RoleLead)
	rec = g.do(http.MethodPatch, projectPath(g.ProjectA.ID), map[string]any{"team_id": g.TeamB.ID}, projectOrigin(), g.as(t, g.LeadA))
	if rec.Code != http.StatusOK {
		t.Fatalf("lead of both teams = %d, want 200: %s", rec.Code, rec.Body)
	}

	// An archived target team takes no projects.
	at := g.clock.now
	archived := &store.Team{Name: "Old", ArchivedAt: &at}
	if err := g.st.CreateTeam(archived); err != nil {
		t.Fatalf("create archived team: %v", err)
	}
	rec = g.do(http.MethodPatch, projectPath(g.ProjectA.ID), map[string]any{"team_id": archived.ID}, projectOrigin(), g.as(t, g.Admin))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("move into an archived team = %d, want 422: %s", rec.Code, rec.Body)
	}
}

func TestProjectArchiveRestore(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()

	rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"archived": true}, origin, f.as(t, f.LeadA))
	if rec.Code != http.StatusOK {
		t.Fatalf("lead archive = %d, want 200: %s", rec.Code, rec.Body)
	}
	p := decode[projectResponse](t, rec)
	if p.ArchivedAt == nil || !p.ArchivedAt.Equal(f.clock.now) {
		t.Fatalf("archived_at = %v, want %v", p.ArchivedAt, f.clock.now)
	}
	if got, err := f.st.ProjectByID(f.ProjectA.ID); err != nil || got.ArchivedAt == nil {
		t.Fatalf("archive not stored: %+v, %v", got, err)
	}
	if list := decode[[]projectResponse](t, f.do(http.MethodGet, "/api/v1/projects", nil, nil, f.as(t, f.Member))); len(list) != 1 || list[0].Name != "Sync" {
		t.Fatalf("list after archive = %+v, want only Sync", list)
	}

	if rec := f.do(http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"archived": false}, origin, f.as(t, f.LeadA)); rec.Code != http.StatusForbidden {
		t.Fatalf("lead unarchive = %d, want 403: %s", rec.Code, rec.Body)
	}
	if rec := f.do(http.MethodPost, projectPath(f.ProjectA.ID)+"/restore", nil, origin, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous restore = %d, want 401", rec.Code)
	}
	for _, u := range []*store.User{f.Member, f.Outsider, f.LeadA} {
		if rec := f.do(http.MethodPost, projectPath(f.ProjectA.ID)+"/restore", nil, origin, f.as(t, u)); rec.Code != http.StatusForbidden {
			t.Errorf("restore as %s = %d, want 403", u.Email, rec.Code)
		}
	}

	rec = f.do(http.MethodPost, projectPath(f.ProjectA.ID)+"/restore", nil, origin, f.as(t, f.Admin))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin restore = %d, want 200: %s", rec.Code, rec.Body)
	}
	if p := decode[projectResponse](t, rec); p.ArchivedAt != nil {
		t.Fatalf("restored archived_at = %v, want nil", p.ArchivedAt)
	}
	if got, err := f.st.ProjectByID(f.ProjectA.ID); err != nil || got.ArchivedAt != nil {
		t.Fatalf("restore not stored: %+v, %v", got, err)
	}

	// Archiving the team archives its projects, and neither restore path may revive
	// one inside a team that grants nothing.
	if rec := f.teamRequest(t, f.Admin, http.MethodDelete, teamPath(fmt.Sprintf("/%d", f.TeamA.ID)), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("archive TeamA = %d, want 204: %s", rec.Code, rec.Body)
	}
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"patch unarchive", http.MethodPatch, projectPath(f.ProjectA.ID), map[string]any{"archived": false}},
		{"restore", http.MethodPost, projectPath(f.ProjectA.ID) + "/restore", nil},
	} {
		rec := f.do(tc.method, tc.path, tc.body, origin, f.as(t, f.Admin))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s in an archived team = %d, want 422: %s", tc.name, rec.Code, rec.Body)
			continue
		}
		if body := decode[struct {
			Type string `json:"type"`
		}](t, rec); body.Type != urnInvalidInput {
			t.Errorf("%s type = %q, want %q", tc.name, body.Type, urnInvalidInput)
		}
	}
	if got, err := f.st.ProjectByID(f.ProjectA.ID); err != nil || got.ArchivedAt == nil {
		t.Fatalf("project revived in an archived team: %+v, %v", got, err)
	}
}

func TestProjectNotFound(t *testing.T) {
	f := seed(t)
	origin := projectOrigin()

	rec := f.do(http.MethodPatch, "/api/v1/projects/9999", map[string]any{"name": "Nope"}, origin, f.as(t, f.Admin))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch unknown = %d, want 404: %s", rec.Code, rec.Body)
	}
	if body := decode[struct {
		Type string `json:"type"`
	}](t, rec); body.Type != urnNotFound {
		t.Fatalf("type = %q, want %q", body.Type, urnNotFound)
	}

	if rec := f.do(http.MethodPost, "/api/v1/projects/9999/restore", nil, origin, f.as(t, f.Admin)); rec.Code != http.StatusNotFound {
		t.Fatalf("restore unknown = %d, want 404: %s", rec.Code, rec.Body)
	}
}
