package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/havspect/tstamps/internal/client"
)

func TestAuditPassesFiltersAndResolvesActor(t *testing.T) {
	f := &fakeClient{
		users: []client.User{{ID: 3, Email: "lead@example.com"}},
		audit: []client.AuditEvent{
			{ID: 2, At: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC),
				ActorUserID: 3, ActorEmail: "lead@example.com", ActorRole: "admin",
				Action: "project.archive", TargetType: "project", TargetID: 4},
			{ID: 1, At: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
				ActorUserID: 3, ActorEmail: "lead@example.com", ActorRole: "admin",
				Action: "project.archive", TargetType: "project", TargetID: 4},
		},
	}
	a, out, errOut := berlinApp(f)

	code := a.Run([]string{"audit", "--from", "2026-10-01", "--to", "2026-11-01",
		"--actor", "lead@example.com", "--action", "project.archive",
		"--target-type", "project", "--target-id", "4", "--limit", "10"})
	if code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	want := client.AuditQuery{
		From: "2026-10-01", To: "2026-11-01", ActorUserID: 3,
		Action: "project.archive", TargetType: "project", TargetID: 4, Limit: 10,
	}
	if f.auditQ != want {
		t.Fatalf("query %+v, want %+v", f.auditQ, want)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout %q, want a header and one row per event", out.String())
	}
	for _, want := range []string{"AT", "ACTOR", "ROLE", "ACTION", "TARGET",
		"2026-10-20 11:30", "lead@example.com", "admin", "project.archive", "project/4"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout %q does not contain %q", out.String(), want)
		}
	}
}

func TestAuditJSON(t *testing.T) {
	f := &fakeClient{audit: []client.AuditEvent{{
		ID: 2, At: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC),
		ActorUserID: 3, ActorEmail: "lead@example.com", ActorRole: "admin",
		Action: "user.update", TargetType: "user", TargetID: 7,
		Changes: json.RawMessage(`{"name":{"from":"a","to":"b"}}`),
	}}}
	a, out, errOut := newTestApp(f)

	if code := a.Run([]string{"audit", "--json"}); code != 0 {
		t.Fatalf("exit %d, want 0 (stderr %q)", code, errOut.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", out.String(), err)
	}
	if len(got) != 1 || got[0]["action"] != "user.update" || got[0]["target_id"] != float64(7) {
		t.Fatalf("JSON %v, want the event", got)
	}
	if _, ok := got[0]["changes"].(map[string]any); !ok {
		t.Fatalf("JSON %v does not carry changes as an object", got)
	}
}

func TestAuditClientErrorExits1(t *testing.T) {
	f := &fakeClient{auditErr: errors.New("forbidden")}
	a, _, errOut := newTestApp(f)

	if code := a.Run([]string{"audit"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "tstamps audit: forbidden") {
		t.Fatalf("stderr %q does not name the failure", errOut.String())
	}
}
