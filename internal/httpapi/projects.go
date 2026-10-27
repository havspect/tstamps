package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/project"
)

type projectResponse struct {
	ID         uint       `json:"id"`
	TeamID     uint       `json:"team_id"`
	Name       string     `json:"name"`
	Color      string     `json:"color"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func toProjectResponse(p *project.Project) projectResponse {
	if p == nil {
		return projectResponse{}
	}
	return projectResponse{
		ID:         p.ID,
		TeamID:     p.TeamID,
		Name:       p.Name,
		Color:      p.Color,
		ArchivedAt: p.ArchivedAt,
	}
}

type projectListInput struct {
	All bool `query:"all" doc:"Include archived projects (admin only)"`
}

type projectListOutput struct {
	Body []projectResponse
}

type projectCreateInput struct {
	Body struct {
		TeamID uint   `json:"team_id" required:"true"`
		Name   string `json:"name" required:"true"`
		Color  string `json:"color,omitempty"`
	}
}

type projectCreateOutput struct {
	Body projectResponse
}

type projectUpdateInput struct {
	ID   uint `path:"id"`
	Body struct {
		TeamID   *uint   `json:"team_id,omitempty"`
		Name     *string `json:"name,omitempty"`
		Color    *string `json:"color,omitempty"`
		Archived *bool   `json:"archived,omitempty"`
	}
}

type projectUpdateOutput struct {
	Body projectResponse
}

type projectArchiveInput struct {
	ID uint `path:"id"`
}

type projectRestoreInput struct {
	ID uint `path:"id"`
}

func (s *Server) projectRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "projects-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects",
		Tags:        []string{"projects"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
	}, s.listProjects)

	huma.Register(api, huma.Operation{
		OperationID:   "projects-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects",
		Tags:          []string{"projects"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createProject)

	huma.Register(api, huma.Operation{
		OperationID: "projects-update",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/projects/{id}",
		Tags:        []string{"projects"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.updateProject)

	huma.Register(api, huma.Operation{
		OperationID:   "projects-archive",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{id}",
		Tags:          []string{"projects"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.archiveProject)

	huma.Register(api, huma.Operation{
		OperationID: "projects-restore",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/projects/{id}/restore",
		Tags:        []string{"projects"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.restoreProject)
}

func (s *Server) listProjects(ctx context.Context, in *projectListInput) (*projectListOutput, error) {
	if s.deps.ProjectService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "project service not available")
	}

	projs, err := s.deps.ProjectService.ListProjects(ctx, in.All)
	if err != nil {
		return nil, fail(err)
	}

	out := make([]projectResponse, len(projs))
	for i, p := range projs {
		out[i] = toProjectResponse(&p)
	}
	return &projectListOutput{Body: out}, nil
}

func (s *Server) createProject(ctx context.Context, in *projectCreateInput) (*projectCreateOutput, error) {
	if s.deps.ProjectService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "project service not available")
	}

	p, err := s.deps.ProjectService.CreateProject(ctx, in.Body.TeamID, in.Body.Name, in.Body.Color)
	if err != nil {
		return nil, fail(err)
	}

	return &projectCreateOutput{Body: toProjectResponse(p)}, nil
}

func (s *Server) updateProject(ctx context.Context, in *projectUpdateInput) (*projectUpdateOutput, error) {
	if s.deps.ProjectService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "project service not available")
	}

	p, err := s.deps.ProjectService.UpdateProject(ctx, in.ID, in.Body.TeamID, in.Body.Name, in.Body.Color, in.Body.Archived)
	if err != nil {
		return nil, fail(err)
	}

	return &projectUpdateOutput{Body: toProjectResponse(p)}, nil
}

func (s *Server) archiveProject(ctx context.Context, in *projectArchiveInput) (*struct{}, error) {
	if s.deps.ProjectService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "project service not available")
	}

	err := s.deps.ProjectService.ArchiveProject(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}

func (s *Server) restoreProject(ctx context.Context, in *projectRestoreInput) (*projectUpdateOutput, error) {
	if s.deps.ProjectService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "project service not available")
	}

	p, err := s.deps.ProjectService.RestoreProject(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &projectUpdateOutput{Body: toProjectResponse(p)}, nil
}
