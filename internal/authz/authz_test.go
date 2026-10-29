package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
)

func TestPolicyEngine_Interface(t *testing.T) {
	var _ authz.Authorizer = authz.NewPolicyEngine()
	var _ authz.Authorizer = &authz.PolicyEngine{}
}

func TestPolicyEngine_Unauthorized(t *testing.T) {
	engine := authz.NewPolicyEngine()

	// Direct nil principal and empty context
	err := engine.Check(context.Background(), nil, authz.ActionLogEntry, nil)
	if !errors.Is(err, authz.ErrUnauthorized) {
		t.Fatalf("Check(nil principal) err = %v, want ErrUnauthorized", err)
	}

	// Principal in context
	p := &auth.Principal{UserID: 1, Roles: []string{"member"}}
	ctx := auth.WithPrincipal(context.Background(), p)
	err = engine.Check(ctx, nil, authz.ActionLogEntry, nil)
	if err != nil {
		t.Fatalf("Check(ctx with principal) err = %v, want nil", err)
	}
}

func TestPolicyEngine_Scopes(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	t.Run("read-only token attempting write actions", func(t *testing.T) {
		p := &auth.Principal{
			UserID: 1,
			Roles:  []string{"admin"},
			Scopes: []string{"tstamps.read"},
		}

		writeActions := []authz.Action{
			authz.ActionLogEntry,
			authz.ActionModifyEntry,
			authz.ActionDeleteEntry,
			authz.ActionRestoreEntry,
			authz.ActionManageProject,
			authz.ActionRestoreProject,
			authz.ActionManageTeam,
			authz.ActionManageUsers,
		}

		for _, action := range writeActions {
			err := engine.Check(ctx, p, action, nil)
			if !errors.Is(err, authz.ErrReadOnly) {
				t.Errorf("Check(%s) err = %v, want ErrReadOnly", action, err)
			}
		}
	})

	t.Run("write token attempting write actions", func(t *testing.T) {
		p := &auth.Principal{
			UserID: 1,
			Roles:  []string{"admin"},
			Scopes: []string{"tstamps.write"},
		}

		// Admin with write scope should be allowed for admin write actions
		err := engine.Check(ctx, p, authz.ActionManageTeam, nil)
		if err != nil {
			t.Errorf("Check(ActionManageTeam) err = %v, want nil", err)
		}
	})

	t.Run("read-only token attempting view actions", func(t *testing.T) {
		p := &auth.Principal{
			UserID: 1,
			Roles:  []string{"member"},
			Scopes: []string{"tstamps.read"},
		}

		err := engine.Check(ctx, p, authz.ActionViewProject, authz.ProjectResource{Archived: false})
		if err != nil {
			t.Errorf("Check(ActionViewProject) err = %v, want nil", err)
		}
	})

	t.Run("unrelated scope attempting view actions", func(t *testing.T) {
		p := &auth.Principal{
			UserID: 1,
			Roles:  []string{"member"},
			Scopes: []string{"unrelated.scope"},
		}

		err := engine.Check(ctx, p, authz.ActionViewProject, authz.ProjectResource{Archived: false})
		if !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("Check(ActionViewProject) err = %v, want ErrForbidden", err)
		}
	})
}

func TestPolicyEngine_EntryModifyAndDelete(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	member := &auth.Principal{UserID: 10, Roles: []string{"member"}}
	admin := &auth.Principal{UserID: 99, Roles: []string{"admin"}}

	actions := []authz.Action{authz.ActionModifyEntry, authz.ActionDeleteEntry}

	for _, action := range actions {
		t.Run(string(action)+" self-ownership", func(t *testing.T) {
			// Owner allowed
			res := authz.EntryResource{UserID: 10, ProjectID: 1}
			if err := engine.Check(ctx, member, action, res); err != nil {
				t.Errorf("owner %s failed: %v", action, err)
			}
			// Pointer receiver allowed
			if err := engine.Check(ctx, member, action, &res); err != nil {
				t.Errorf("owner %s (pointer) failed: %v", action, err)
			}

			// Another member forbidden
			otherRes := authz.EntryResource{UserID: 20, ProjectID: 1}
			if err := engine.Check(ctx, member, action, otherRes); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("non-owner %s err = %v, want ErrForbidden", action, err)
			}

			// Admin forbidden for other user's entry (strictly self-ownership!)
			if err := engine.Check(ctx, admin, action, res); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("admin %s on other's entry err = %v, want ErrForbidden", action, err)
			}

			// Admin allowed for admin's own entry
			adminOwnRes := authz.EntryResource{UserID: 99, ProjectID: 1}
			if err := engine.Check(ctx, admin, action, adminOwnRes); err != nil {
				t.Errorf("admin %s on own entry failed: %v", action, err)
			}

			// Missing resource forbidden
			if err := engine.Check(ctx, member, action, nil); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("nil resource %s err = %v, want ErrForbidden", action, err)
			}

			// Zero user ID resource forbidden
			zeroRes := authz.EntryResource{UserID: 0, ProjectID: 1}
			if err := engine.Check(ctx, member, action, zeroRes); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("zero user ID resource %s err = %v, want ErrForbidden", action, err)
			}
		})
	}
}

func TestPolicyEngine_EntryRestore(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	res := authz.EntryResource{UserID: 3, ProjectID: 1}

	// Admin allowed
	if err := engine.Check(ctx, admin, authz.ActionRestoreEntry, res); err != nil {
		t.Errorf("admin restore entry failed: %v", err)
	}

	// Lead forbidden
	if err := engine.Check(ctx, lead, authz.ActionRestoreEntry, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead restore entry err = %v, want ErrForbidden", err)
	}

	// Member forbidden (even for own entry)
	if err := engine.Check(ctx, member, authz.ActionRestoreEntry, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member restore own entry err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_LogEntry(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	member := &auth.Principal{UserID: 10, Roles: []string{"member"}}

	// No resource specified -> allowed
	if err := engine.Check(ctx, member, authz.ActionLogEntry, nil); err != nil {
		t.Errorf("log entry with nil resource failed: %v", err)
	}

	// Resource matching member UserID -> allowed
	res := authz.EntryResource{UserID: 10, ProjectID: 1}
	if err := engine.Check(ctx, member, authz.ActionLogEntry, res); err != nil {
		t.Errorf("log entry for self failed: %v", err)
	}

	// Resource with different UserID -> forbidden
	otherRes := authz.EntryResource{UserID: 20, ProjectID: 1}
	if err := engine.Check(ctx, member, authz.ActionLogEntry, otherRes); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("log entry for another user err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_ProjectManage(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{10, 20}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	team10Project := authz.ProjectResource{TeamID: 10}
	team30Project := authz.ProjectResource{TeamID: 30}

	// Admin allowed for any team project
	if err := engine.Check(ctx, admin, authz.ActionManageProject, team10Project); err != nil {
		t.Errorf("admin manage project failed: %v", err)
	}
	if err := engine.Check(ctx, admin, authz.ActionManageProject, team30Project); err != nil {
		t.Errorf("admin manage project failed: %v", err)
	}

	// Lead allowed for led team
	if err := engine.Check(ctx, lead, authz.ActionManageProject, team10Project); err != nil {
		t.Errorf("lead manage project for led team failed: %v", err)
	}
	if err := engine.Check(ctx, lead, authz.ActionManageProject, &team10Project); err != nil {
		t.Errorf("lead manage project (pointer) for led team failed: %v", err)
	}

	// Lead managing TeamResource directly
	team10Res := authz.TeamResource{TeamID: 10}
	if err := engine.Check(ctx, lead, authz.ActionManageProject, team10Res); err != nil {
		t.Errorf("lead manage project with TeamResource failed: %v", err)
	}

	// Lead forbidden for non-led team
	if err := engine.Check(ctx, lead, authz.ActionManageProject, team30Project); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead manage project for other team err = %v, want ErrForbidden", err)
	}

	// Member forbidden
	if err := engine.Check(ctx, member, authz.ActionManageProject, team10Project); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member manage project err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_ProjectRestore(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{10}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	res := authz.ProjectResource{TeamID: 10, Archived: true}

	if err := engine.Check(ctx, admin, authz.ActionRestoreProject, res); err != nil {
		t.Errorf("admin restore project failed: %v", err)
	}
	if err := engine.Check(ctx, lead, authz.ActionRestoreProject, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead restore project err = %v, want ErrForbidden", err)
	}
	if err := engine.Check(ctx, member, authz.ActionRestoreProject, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member restore project err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_ProjectView(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	member := &auth.Principal{UserID: 2, Roles: []string{"member"}}

	activeProject := authz.ProjectResource{TeamID: 5, Archived: false}
	archivedProject := authz.ProjectResource{TeamID: 5, Archived: true}

	// Active project viewable by all
	if err := engine.Check(ctx, member, authz.ActionViewProject, activeProject); err != nil {
		t.Errorf("member view active project failed: %v", err)
	}
	if err := engine.Check(ctx, admin, authz.ActionViewProject, activeProject); err != nil {
		t.Errorf("admin view active project failed: %v", err)
	}

	// Archived project viewable by admin only
	if err := engine.Check(ctx, admin, authz.ActionViewProject, archivedProject); err != nil {
		t.Errorf("admin view archived project failed: %v", err)
	}
	if err := engine.Check(ctx, member, authz.ActionViewProject, archivedProject); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member view archived project err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_TeamManage(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{10}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	res := authz.TeamResource{TeamID: 10}

	if err := engine.Check(ctx, admin, authz.ActionManageTeam, res); err != nil {
		t.Errorf("admin manage team failed: %v", err)
	}
	if err := engine.Check(ctx, lead, authz.ActionManageTeam, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead manage team err = %v, want ErrForbidden", err)
	}
	if err := engine.Check(ctx, member, authz.ActionManageTeam, res); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member manage team err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_TeamView(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{10}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	ledTeam := authz.TeamResource{TeamID: 10}
	otherTeam := authz.TeamResource{TeamID: 20}

	// Admin allowed
	if err := engine.Check(ctx, admin, authz.ActionViewTeam, ledTeam); err != nil {
		t.Errorf("admin view team failed: %v", err)
	}

	// Lead viewing led team allowed
	if err := engine.Check(ctx, lead, authz.ActionViewTeam, ledTeam); err != nil {
		t.Errorf("lead view led team failed: %v", err)
	}

	// Lead viewing other team forbidden
	if err := engine.Check(ctx, lead, authz.ActionViewTeam, otherTeam); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead view other team err = %v, want ErrForbidden", err)
	}

	// Member forbidden
	if err := engine.Check(ctx, member, authz.ActionViewTeam, ledTeam); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("member view team err = %v, want ErrForbidden", err)
	}
}

func TestPolicyEngine_UsersAndAudit(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	adminOnlyActions := []authz.Action{
		authz.ActionManageUsers,
		authz.ActionViewUsers,
		authz.ActionViewAudit,
	}

	for _, action := range adminOnlyActions {
		if err := engine.Check(ctx, admin, action, nil); err != nil {
			t.Errorf("admin %s failed: %v", action, err)
		}
		if err := engine.Check(ctx, lead, action, nil); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("lead %s err = %v, want ErrForbidden", action, err)
		}
		if err := engine.Check(ctx, member, action, nil); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("member %s err = %v, want ErrForbidden", action, err)
		}
	}
}

func TestPolicyEngine_ReportView(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()

	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}
	lead := &auth.Principal{UserID: 2, Roles: []string{"lead"}, LedTeamIDs: []uint{10}}
	member := &auth.Principal{UserID: 3, Roles: []string{"member"}}

	// Admin allowed
	if err := engine.Check(ctx, admin, authz.ActionViewReport, nil); err != nil {
		t.Errorf("admin view report failed: %v", err)
	}

	// Lead allowed
	if err := engine.Check(ctx, lead, authz.ActionViewReport, nil); err != nil {
		t.Errorf("lead view report failed: %v", err)
	}

	// Lead with led team allowed
	if err := engine.Check(ctx, lead, authz.ActionViewReport, authz.TeamResource{TeamID: 10}); err != nil {
		t.Errorf("lead view report for led team failed: %v", err)
	}

	// Lead with other team forbidden
	if err := engine.Check(ctx, lead, authz.ActionViewReport, authz.TeamResource{TeamID: 20}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("lead view report for other team err = %v, want ErrForbidden", err)
	}

	// Member allowed: the report service narrows the rows to their own entries
	if err := engine.Check(ctx, member, authz.ActionViewReport, nil); err != nil {
		t.Errorf("member view report failed: %v", err)
	}
}

func TestPolicyEngine_UnknownAction(t *testing.T) {
	engine := authz.NewPolicyEngine()
	ctx := context.Background()
	admin := &auth.Principal{UserID: 1, Roles: []string{"admin"}}

	if err := engine.Check(ctx, admin, authz.Action("unknown:action"), nil); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("unknown action err = %v, want ErrForbidden", err)
	}
}
