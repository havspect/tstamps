package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

type teamMemberResponse struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email,omitempty"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role"`
}

type teamResponse struct {
	ID         uint                 `json:"id"`
	Name       string               `json:"name"`
	ArchivedAt *time.Time           `json:"archived_at"`
	Members    []teamMemberResponse `json:"members"`
}

type teamListInput struct {
	All bool `query:"all" doc:"Include archived teams (admin only)"`
}

type teamListOutput struct {
	Body []teamResponse
}

type teamCreateInput struct {
	Body struct {
		Name string `json:"name" required:"true"`
	}
}

type teamCreateOutput struct {
	Body teamResponse
}

type teamRenameInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name string `json:"name" required:"true"`
	}
}

type teamRenameOutput struct {
	Body teamResponse
}

type teamArchiveInput struct {
	ID uint `path:"id"`
}

type teamMemberInput struct {
	UserID uint   `json:"user_id" required:"true"`
	Role   string `json:"role" required:"true" enum:"lead,member"`
}

type teamReplaceMembersInput struct {
	ID   uint              `path:"id"`
	Body []teamMemberInput `json:"body"`
}

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
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.archiveTeam)

	huma.Register(api, huma.Operation{
		OperationID:   "teams-replace-members",
		Method:        http.MethodPut,
		Path:          apiPrefix + "/teams/{id}/members",
		Tags:          []string{"teams"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.replaceMembers)
}

func (s *Server) listTeams(ctx context.Context, in *teamListInput) (*teamListOutput, error) {
	if s.deps.TeamService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "team service not available")
	}

	teams, err := s.deps.TeamService.ListTeams(ctx, in.All)
	if err != nil {
		return nil, fail(err)
	}

	usersMap := make(map[uint]user.User)
	if s.deps.UserService != nil {
		if users, err := s.deps.UserService.ListUsers(ctx); err == nil {
			for _, u := range users {
				usersMap[u.ID] = u
			}
		}
	}

	out := make([]teamResponse, len(teams))
	for i, t := range teams {
		members := make([]teamMemberResponse, len(t.Members))
		for j, m := range t.Members {
			u := usersMap[m.UserID]
			members[j] = teamMemberResponse{
				UserID: m.UserID,
				Email:  u.Email,
				Name:   u.Name,
				Role:   m.Role,
			}
		}
		out[i] = teamResponse{
			ID:         t.Team.ID,
			Name:       t.Team.Name,
			ArchivedAt: t.Team.ArchivedAt,
			Members:    members,
		}
	}
	return &teamListOutput{Body: out}, nil
}

func (s *Server) createTeam(ctx context.Context, in *teamCreateInput) (*teamCreateOutput, error) {
	if s.deps.TeamService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "team service not available")
	}

	t, err := s.deps.TeamService.CreateTeam(ctx, in.Body.Name)
	if err != nil {
		return nil, fail(err)
	}

	return &teamCreateOutput{
		Body: teamResponse{
			ID:         t.ID,
			Name:       t.Name,
			ArchivedAt: t.ArchivedAt,
			Members:    []teamMemberResponse{},
		},
	}, nil
}

func (s *Server) renameTeam(ctx context.Context, in *teamRenameInput) (*teamRenameOutput, error) {
	if s.deps.TeamService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "team service not available")
	}

	t, err := s.deps.TeamService.RenameTeam(ctx, in.ID, in.Body.Name)
	if err != nil {
		return nil, fail(err)
	}

	return &teamRenameOutput{
		Body: teamResponse{
			ID:         t.ID,
			Name:       t.Name,
			ArchivedAt: t.ArchivedAt,
			Members:    []teamMemberResponse{},
		},
	}, nil
}

func (s *Server) archiveTeam(ctx context.Context, in *teamArchiveInput) (*struct{}, error) {
	if s.deps.TeamService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "team service not available")
	}

	err := s.deps.TeamService.ArchiveTeam(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}

func (s *Server) replaceMembers(ctx context.Context, in *teamReplaceMembersInput) (*struct{}, error) {
	if s.deps.TeamService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "team service not available")
	}

	domMembers := make([]team.MembershipInput, len(in.Body))
	for i, m := range in.Body {
		domMembers[i] = team.MembershipInput{
			UserID: m.UserID,
			Role:   m.Role,
		}
	}

	err := s.deps.TeamService.ReplaceMembers(ctx, in.ID, domMembers)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}
