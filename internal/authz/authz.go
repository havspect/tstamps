// Package authz provides the policy decision point and policy enforcement point for workspace operations.
package authz

import (
	"context"
	"errors"

	"github.com/havspect/tstamps/internal/auth"
)

var (
	// ErrForbidden indicates the principal is not allowed to perform the requested action on the resource.
	ErrForbidden = errors.New("forbidden")

	// ErrReadOnly indicates a write operation was attempted using a token without write scope.
	ErrReadOnly = errors.New("write operation forbidden with read-only token")

	// ErrUnauthorized indicates no valid authenticated principal was supplied.
	ErrUnauthorized = errors.New("unauthorized")
)

// Action identifies a domain operation subjected to authorization policies.
type Action string

const (
	ActionLogEntry       Action = "ActionLogEntry"
	ActionModifyEntry    Action = "ActionModifyEntry"
	ActionDeleteEntry    Action = "ActionDeleteEntry"
	ActionRestoreEntry   Action = "ActionRestoreEntry"
	ActionManageProject  Action = "ActionManageProject"
	ActionRestoreProject Action = "ActionRestoreProject"
	ActionViewProject    Action = "ActionViewProject"
	ActionManageTeam     Action = "ActionManageTeam"
	ActionViewTeam       Action = "ActionViewTeam"
	ActionManageUsers    Action = "ActionManageUsers"
	ActionViewUsers      Action = "ActionViewUsers"
	ActionViewAudit      Action = "ActionViewAudit"
	ActionViewReport     Action = "ActionViewReport"
)

// IsWrite reports whether the action mutates state.
func (a Action) IsWrite() bool {
	switch a {
	case ActionLogEntry, "entry:log",
		ActionModifyEntry, "entry:modify", "entry:update",
		ActionDeleteEntry, "entry:delete",
		ActionRestoreEntry, "entry:restore",
		ActionManageProject, "project:manage",
		ActionRestoreProject, "project:restore",
		ActionManageTeam, "team:manage",
		ActionManageUsers, "users:manage", "user:manage":
		return true
	default:
		return false
	}
}

// EntryResource models an entry target for authorization checks.
type EntryResource struct {
	UserID    uint
	ProjectID uint
}

// ProjectResource models a project target for authorization checks.
type ProjectResource struct {
	TeamID   uint
	Archived bool
}

// TeamResource models a team target for authorization checks.
type TeamResource struct {
	TeamID uint
}

// Authorizer decides whether a principal may perform an action on a resource within a context.
type Authorizer interface {
	Check(ctx context.Context, p *auth.Principal, action Action, res any) error
}

// PolicyEngine is the default Authorizer implementation enforcing workspace policy decisions.
type PolicyEngine struct{}

// NewPolicyEngine returns an initialized PolicyEngine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

var _ Authorizer = (*PolicyEngine)(nil)

func toEntryResource(res any) (EntryResource, bool) {
	switch r := res.(type) {
	case EntryResource:
		return r, true
	case *EntryResource:
		if r != nil {
			return *r, true
		}
	}
	return EntryResource{}, false
}

func toProjectResource(res any) (ProjectResource, bool) {
	switch r := res.(type) {
	case ProjectResource:
		return r, true
	case *ProjectResource:
		if r != nil {
			return *r, true
		}
	}
	return ProjectResource{}, false
}

func toTeamResource(res any) (TeamResource, bool) {
	switch r := res.(type) {
	case TeamResource:
		return r, true
	case *TeamResource:
		if r != nil {
			return *r, true
		}
	}
	return TeamResource{}, false
}

// Check evaluates whether principal p is authorized to perform action on resource res.
func (e *PolicyEngine) Check(ctx context.Context, p *auth.Principal, action Action, res any) error {
	if p == nil && ctx != nil {
		p, _ = auth.PrincipalFrom(ctx)
	}
	if p == nil {
		return ErrUnauthorized
	}

	// Scope enforcement: if principal has scopes, write operations require tstamps.write,
	// and read operations require tstamps.read or tstamps.write.
	if len(p.Scopes) > 0 {
		if action.IsWrite() {
			if !p.HasScope("tstamps.write") {
				return ErrReadOnly
			}
		} else {
			if !p.HasScope("tstamps.read") && !p.HasScope("tstamps.write") {
				return ErrForbidden
			}
		}
	}

	switch action {
	case ActionLogEntry, "entry:log":
		if er, ok := toEntryResource(res); ok {
			if er.UserID != 0 && er.UserID != p.UserID {
				return ErrForbidden
			}
		}
		return nil

	case ActionModifyEntry, "entry:modify", "entry:update",
		ActionDeleteEntry, "entry:delete":
		// Strictly requires self-ownership (p.UserID == res.UserID).
		// Admin cannot modify or delete other users' entries.
		er, ok := toEntryResource(res)
		if !ok || er.UserID == 0 || er.UserID != p.UserID {
			return ErrForbidden
		}
		return nil

	case ActionRestoreEntry, "entry:restore":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionManageProject, "project:manage":
		if p.IsAdmin() {
			return nil
		}
		if p.HasRole("lead") {
			if pr, ok := toProjectResource(res); ok && p.LeadsTeam(pr.TeamID) {
				return nil
			}
			if tr, ok := toTeamResource(res); ok && p.LeadsTeam(tr.TeamID) {
				return nil
			}
		}
		return ErrForbidden

	case ActionRestoreProject, "project:restore":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionViewProject, "project:view":
		if pr, ok := toProjectResource(res); ok && pr.Archived {
			if !p.IsAdmin() {
				return ErrForbidden
			}
		}
		return nil

	case ActionManageTeam, "team:manage":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionViewTeam, "team:view":
		if p.IsAdmin() {
			return nil
		}
		if p.HasRole("lead") {
			if tr, ok := toTeamResource(res); ok && tr.TeamID != 0 {
				if p.LeadsTeam(tr.TeamID) {
					return nil
				}
				return ErrForbidden
			}
			return nil
		}
		return ErrForbidden

	case ActionManageUsers, "users:manage", "user:manage":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionViewUsers, "users:view", "user:view":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionViewAudit, "audit:view":
		if !p.IsAdmin() {
			return ErrForbidden
		}
		return nil

	case ActionViewReport, "report:view":
		if p.IsAdmin() {
			return nil
		}
		if p.HasRole("lead") {
			if tr, ok := toTeamResource(res); ok && tr.TeamID != 0 && !p.LeadsTeam(tr.TeamID) {
				return ErrForbidden
			}
			if pr, ok := toProjectResource(res); ok && pr.TeamID != 0 && !p.LeadsTeam(pr.TeamID) {
				return ErrForbidden
			}
			return nil
		}
		return ErrForbidden

	default:
		return ErrForbidden
	}
}
