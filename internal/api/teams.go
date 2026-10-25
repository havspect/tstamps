package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/store"
)

// teamRoutes registers the team and membership endpoints.
func (s *Server) teamRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "teams-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/teams",
		Tags:        []string{"teams"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	}, s.listTeams)

	huma.Register(api, huma.Operation{
		OperationID:   "teams-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/teams",
		Tags:          []string{"teams"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createTeam)

	huma.Register(api, huma.Operation{
		OperationID: "teams-rename",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/teams/{id}",
		Tags:        []string{"teams"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.renameTeam)

	huma.Register(api, huma.Operation{
		OperationID:   "teams-archive",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/teams/{id}",
		Tags:          []string{"teams"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.archiveTeam)

	huma.Register(api, huma.Operation{
		OperationID: "teams-members-replace",
		Method:      http.MethodPut,
		Path:        apiPrefix + "/teams/{id}/members",
		Tags:        []string{"teams"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.replaceMembers)
}

type teamMemberResponse struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

type teamResponse struct {
	ID         uint                 `json:"id"`
	Name       string               `json:"name"`
	ArchivedAt *time.Time           `json:"archived_at"`
	Members    []teamMemberResponse `json:"members"`
}

type teamMemberInput struct {
	UserID uint   `json:"user_id" required:"true"`
	Role   string `json:"role" required:"true" enum:"lead,member"`
}

type teamNameInput struct {
	Body struct {
		Name string `json:"name" required:"true" minLength:"1"`
	}
}

type teamRenameInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name string `json:"name" required:"true" minLength:"1"`
	}
}

type teamIDInput struct {
	ID uint `path:"id"`
}

type teamMembersInput struct {
	ID   uint `path:"id"`
	Body []teamMemberInput
}

type teamsOutput struct{ Body []teamResponse }

type teamOutput struct{ Body teamResponse }

type teamMembersOutput struct{ Body []teamMemberResponse }

func teamOf(t store.Team, members []store.Member) teamResponse {
	out := teamResponse{
		ID: t.ID, Name: t.Name, ArchivedAt: t.ArchivedAt,
		Members: []teamMemberResponse{},
	}
	for _, m := range members {
		out.Members = append(out.Members, teamMemberOf(m))
	}
	return out
}

func teamMemberOf(m store.Member) teamMemberResponse {
	return teamMemberResponse{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role}
}

func (s *Server) listTeams(ctx context.Context, _ *struct{}) (*teamsOutput, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	teams, err := s.Store.ListTeams()
	if err != nil {
		return nil, internal(err)
	}
	all, err := s.Store.AllMembers()
	if err != nil {
		return nil, internal(err)
	}
	byTeam := make(map[uint][]store.Member, len(teams))
	for _, m := range all {
		byTeam[m.TeamID] = append(byTeam[m.TeamID], m)
	}
	out := make([]teamResponse, 0, len(teams))
	for _, t := range teams {
		out = append(out, teamOf(t, byTeam[t.ID]))
	}
	return &teamsOutput{Body: out}, nil
}

func (s *Server) createTeam(ctx context.Context, in *teamNameInput) (*teamOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	team := &store.Team{Name: in.Body.Name}
	now := s.Clock.Now()
	err = s.Store.InTx(func(tx *store.Store) error {
		if err := tx.CreateTeam(team); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "team.create", "team", team.ID,
			store.Change{After: team.AuditFields()})
	})
	if err != nil {
		return nil, internal(err)
	}
	return &teamOutput{Body: teamOf(*team, nil)}, nil
}

func (s *Server) renameTeam(ctx context.Context, in *teamRenameInput) (*teamOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	var team *store.Team
	var members []store.Member
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.TeamByID(in.ID)
		if err != nil {
			return err
		}
		if team, err = tx.RenameTeam(in.ID, in.Body.Name, now); err != nil {
			return err
		}
		if members, err = tx.MembersOf(in.ID); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "team.rename", "team", in.ID,
			store.Change{Before: before.AuditFields(), After: team.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &teamOutput{Body: teamOf(*team, members)}, nil
}

func (s *Server) archiveTeam(ctx context.Context, in *teamIDInput) (*struct{}, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.TeamByID(in.ID)
		if err != nil {
			return err
		}
		// This is exactly the set ArchiveTeam will archive below.
		var projects []store.Project
		if before.ArchivedAt == nil {
			all, err := tx.Projects(true)
			if err != nil {
				return err
			}
			for _, p := range all {
				if p.TeamID == in.ID && p.ArchivedAt == nil {
					projects = append(projects, p)
				}
			}
		}
		if err := tx.ArchiveTeam(in.ID, now); err != nil {
			return err
		}
		if before.ArchivedAt != nil {
			return nil
		}
		actor := s.actorOf(ctx, admin)
		after, err := tx.TeamByID(in.ID)
		if err != nil {
			return err
		}
		if err := tx.RecordAudit(now, actor, "team.archive", "team", in.ID,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()}); err != nil {
			return err
		}
		for _, p := range projects {
			afterProject, err := tx.ProjectByID(p.ID)
			if err != nil {
				return err
			}
			if err := tx.RecordAudit(now, actor, "project.archive", "project", p.ID,
				store.Change{Before: p.AuditFields(), After: afterProject.AuditFields()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fail(err)
	}
	return nil, nil
}

func (s *Server) replaceMembers(ctx context.Context, in *teamMembersInput) (*teamMembersOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[uint]bool, len(in.Body))
	members := make([]store.MemberRole, 0, len(in.Body))
	for _, m := range in.Body {
		if seen[m.UserID] {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "the same user_id appears twice")
		}
		seen[m.UserID] = true
		members = append(members, store.MemberRole{UserID: m.UserID, Role: m.Role})
	}
	// A membership list is not a row, so before and after are built from it.
	membershipList := func(rows []store.Member) map[string]any {
		list := make([]map[string]any, 0, len(rows))
		for _, m := range rows {
			list = append(list, map[string]any{"user_id": m.UserID, "email": m.Email, "role": m.Role})
		}
		return map[string]any{"members": list}
	}
	now := s.Clock.Now()
	var after []store.Member
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.MembersOf(in.ID)
		if err != nil {
			return err
		}
		if err := tx.ReplaceMemberships(in.ID, members, now); err != nil {
			return err
		}
		if after, err = tx.MembersOf(in.ID); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "team.members.replace", "team", in.ID,
			store.Change{Before: membershipList(before), After: membershipList(after)})
	})
	if err != nil {
		return nil, fail(err)
	}
	out := make([]teamMemberResponse, 0, len(after))
	for _, m := range after {
		out = append(out, teamMemberOf(m))
	}
	return &teamMembersOutput{Body: out}, nil
}
