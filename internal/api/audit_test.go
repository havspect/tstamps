package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/store"
)

// auditEvents reads the whole log straight from the store, which is also the
// endpoint's source.
func auditEvents(t *testing.T, f *fixture) []store.AuditEvent {
	t.Helper()
	events, err := f.st.AuditEvents(store.AuditFilter{})
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	return events
}

func auditChange(t *testing.T, e store.AuditEvent) store.Change {
	t.Helper()
	var c store.Change
	if err := json.Unmarshal([]byte(e.Changes), &c); err != nil {
		t.Fatalf("decode changes %q: %v", e.Changes, err)
	}
	return c
}

func eventFor(t *testing.T, events []store.AuditEvent, action string) store.AuditEvent {
	t.Helper()
	for _, e := range events {
		if e.Action == action {
			return e
		}
	}
	t.Fatalf("no %s event in %+v", action, events)
	return store.AuditEvent{}
}

func TestAuditDeactivationStopsRunningEntry(t *testing.T) {
	f := seed(t)
	running := f.running(t, f.Outsider, f.ProjectA, f.clock.now.Add(-time.Hour), "working")

	rec := sendAs(t, f, f.Admin, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", f.Outsider.ID),
		map[string]any{"active": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	events := auditEvents(t, f)
	if len(events) != 2 {
		t.Fatalf("events = %+v, want user.update and entry.stop", events)
	}
	eventFor(t, events, "user.update")
	stop := eventFor(t, events, "entry.stop")
	if stop.TargetType != "entry" || stop.TargetID != running.ID {
		t.Fatalf("entry.stop target = %s/%d, want entry/%d", stop.TargetType, stop.TargetID, running.ID)
	}
	c := auditChange(t, stop)
	if c.Before["end_at"] != nil {
		t.Fatalf("before.end_at = %v, want null", c.Before["end_at"])
	}
	if c.After["end_at"] == nil {
		t.Fatal("after.end_at is null, want the stop time")
	}
}

func TestAuditTeamArchiveCascades(t *testing.T) {
	f := seed(t)
	f.project(t, f.TeamA, "Docs")

	path := fmt.Sprintf("/api/v1/teams/%d", f.TeamA.ID)
	if rec := sendAs(t, f, f.Admin, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("archive status = %d, want 204: %s", rec.Code, rec.Body)
	}
	events := auditEvents(t, f)
	var teamArchives, projectArchives int
	for _, e := range events {
		switch e.Action {
		case "team.archive":
			teamArchives++
		case "project.archive":
			projectArchives++
		}
	}
	if teamArchives != 1 || projectArchives != 2 {
		t.Fatalf("events = %+v, want 1 team.archive and 2 project.archive", events)
	}

	// A repeat archive changes no row, so it writes no event.
	if rec := sendAs(t, f, f.Admin, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("second archive status = %d, want 204: %s", rec.Code, rec.Body)
	}
	if got := len(auditEvents(t, f)); got != len(events) {
		t.Fatalf("events after the second archive = %d, want %d", got, len(events))
	}
}

func TestAuditLeadProjectCreate(t *testing.T) {
	f := seed(t)
	path := apiPrefix + "/projects"

	rec := sendAs(t, f, f.LeadA, http.MethodPost, path, map[string]any{"team_id": f.TeamA.ID, "name": "Audited"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("lead create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	events := auditEvents(t, f)
	if len(events) != 1 || events[0].Action != "project.create" || events[0].ActorRole != "lead" {
		t.Fatalf("events = %+v, want one project.create by a lead", events)
	}

	rec = sendAs(t, f, f.Member, http.MethodPost, path, map[string]any{"team_id": f.TeamA.ID, "name": "Denied"})
	wantProblem(t, rec, http.StatusForbidden, urnForbidden)
	if got := len(auditEvents(t, f)); got != 1 {
		t.Fatalf("events after the refused create = %d, want 1", got)
	}
}

func TestAuditEntryRestore(t *testing.T) {
	f := seed(t)
	e := f.entry(t, f.Outsider, f.ProjectA, f.clock.now.Add(-2*time.Hour), f.clock.now.Add(-time.Hour), "gone")
	if err := f.st.SoftDeleteEntry(e.ID, f.clock.now); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	rec := sendAs(t, f, f.Admin, http.MethodPost, fmt.Sprintf("/api/v1/entries/%d/restore", e.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	events := auditEvents(t, f)
	if len(events) != 1 || events[0].Action != "entry.restore" || events[0].TargetType != "entry" {
		t.Fatalf("events = %+v, want one entry.restore", events)
	}
	c := auditChange(t, events[0])
	if c.Before["deleted_at"] == nil {
		t.Fatal("before.deleted_at = nil, want the deletion time")
	}
	if c.After["deleted_at"] != nil {
		t.Fatalf("after.deleted_at = %v, want null", c.After["deleted_at"])
	}
}

func TestAuditNeverStoresPasswordHash(t *testing.T) {
	f := seed(t)
	rec := sendAs(t, f, f.Admin, http.MethodPost, apiPrefix+"/users", map[string]any{
		"email": "new@example.com", "name": "New Person", "timezone": "UTC",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body)
	}
	created := decode[createdUser](t, rec)

	if rec := sendAs(t, f, f.Admin, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", created.ID),
		map[string]any{"name": "Renamed"}); rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if rec := sendAs(t, f, f.Admin, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", created.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200: %s", rec.Code, rec.Body)
	}

	u, err := f.st.UserByEmail("new@example.com")
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	events := auditEvents(t, f)
	if len(events) != 3 {
		t.Fatalf("events = %d, want create, update and reset", len(events))
	}
	for _, e := range events {
		if strings.Contains(e.Changes, "password_hash") {
			t.Errorf("%s event names password_hash: %s", e.Action, e.Changes)
		}
		if strings.Contains(e.Changes, u.PasswordHash) {
			t.Errorf("%s event contains the password hash: %s", e.Action, e.Changes)
		}
	}
}

func TestAuditRefusedMutationWritesNothing(t *testing.T) {
	f := seed(t)
	rec := sendAs(t, f, f.Admin, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", f.Admin.ID),
		map[string]any{"is_admin": false})
	wantProblem(t, rec, http.StatusUnprocessableEntity, urnLastAdmin)
	if events := auditEvents(t, f); len(events) != 0 {
		t.Fatalf("events = %+v, want none after a rolled-back mutation", events)
	}
}

func TestAuditList(t *testing.T) {
	f := seed(t)

	created := decode[createdUser](t, sendAs(t, f, f.Admin, http.MethodPost, apiPrefix+"/users", map[string]any{
		"email": "new@example.com", "name": "New Person", "timezone": "UTC",
	}))
	if rec := sendAs(t, f, f.Admin, http.MethodPost, apiPrefix+"/teams", map[string]string{"name": "Design"}); rec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if rec := sendAs(t, f, f.LeadA, http.MethodPost, apiPrefix+"/projects", map[string]any{
		"team_id": f.TeamA.ID, "name": "Audited",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, want 201: %s", rec.Code, rec.Body)
	}

	for _, u := range []*store.User{f.LeadA, f.Member} {
		wantProblem(t, f.doAs(t, u, http.MethodGet, apiPrefix+"/audit", nil), http.StatusForbidden, urnForbidden)
	}

	rec := f.doAs(t, f.Admin, http.MethodGet, apiPrefix+"/audit", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	list := decode[struct {
		Events []auditEventResponse `json:"events"`
	}](t, rec)
	if len(list.Events) != 3 {
		t.Fatalf("events = %+v, want 3", list.Events)
	}
	for i := 1; i < len(list.Events); i++ {
		if list.Events[i-1].ID <= list.Events[i].ID {
			t.Fatalf("events are not newest first: %+v", list.Events)
		}
	}
	for _, e := range list.Events {
		if e.ActorEmail == "" || e.ActorName == "" || e.ActorRole == "" {
			t.Errorf("actor not resolved: %+v", e)
		}
	}

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"?action=project.create", 1},
		{fmt.Sprintf("?actor_user_id=%d", f.Admin.ID), 2},
		{fmt.Sprintf("?target_type=user&target_id=%d", created.ID), 1},
		{"?limit=1", 1},
	} {
		rec := f.doAs(t, f.Admin, http.MethodGet, apiPrefix+"/audit"+tc.query, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200: %s", tc.query, rec.Code, rec.Body)
		}
		got := decode[struct {
			Events []auditEventResponse `json:"events"`
		}](t, rec)
		if len(got.Events) != tc.want {
			t.Fatalf("%s: events = %d, want %d", tc.query, len(got.Events), tc.want)
		}
	}

	wantProblem(t, f.doAs(t, f.Admin, http.MethodGet, apiPrefix+"/audit?from=2026-10-22", nil),
		http.StatusUnprocessableEntity, urnInvalidInput)
}

func TestAuditListEmptyIsArray(t *testing.T) {
	f := seed(t)
	rec := f.doAs(t, f.Admin, http.MethodGet, apiPrefix+"/audit", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("body = %s, want an empty events array", rec.Body)
	}
}

func TestAuditUserUpdateIsOneEvent(t *testing.T) {
	f := seed(t)
	rec := sendAs(t, f, f.Admin, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", f.Outsider.ID),
		map[string]any{"name": "Maggie"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	events := auditEvents(t, f)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want exactly one", events)
	}
	e := events[0]
	if e.Action != "user.update" || e.TargetType != "user" || e.TargetID != f.Outsider.ID {
		t.Fatalf("event = %+v, want user.update on user %d", e, f.Outsider.ID)
	}
	c := auditChange(t, e)
	if c.Before["name"] != "Margaret Hamilton" || c.After["name"] != "Maggie" {
		t.Fatalf("changes = %+v, want the name snapshot", c)
	}
}
