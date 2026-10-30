package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/havspect/tstamps/internal/client"
)

// A plain member gets a 403 from list teams; the CLI says who may list them.
func TestTeamsForbiddenExplainsWhoMayList(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"teams", []string{"teams"}, "tstamps teams: listing teams needs an admin or a team lead"},
		{"members", []string{"teams", "members", "Platform"}, "tstamps teams members: listing teams needs an admin or a team lead"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeClient{teamsErr: &client.Error{
				StatusCode: http.StatusForbidden,
				Type:       "urn:tstamps:error:forbidden",
				Detail:     "forbidden",
			}}
			a, _, errOut := newTestApp(f)

			if code := a.Run(tc.args); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if got := strings.TrimSpace(errOut.String()); got != tc.want {
				t.Fatalf("stderr %q, want %q", got, tc.want)
			}
		})
	}
}
