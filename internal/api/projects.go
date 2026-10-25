package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/store"
)

// projectResponse is the shape of a project in every project reply.
type projectResponse struct {
	ID         uint       `json:"id"`
	TeamID     uint       `json:"team_id"`
	Name       string     `json:"name"`
	Color      string     `json:"color"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func projectResponseOf(p *store.Project) projectResponse {
	return projectResponse{ID: p.ID, TeamID: p.TeamID, Name: p.Name, Color: p.Color, ArchivedAt: p.ArchivedAt}
}

// projectRoutes registers the project endpoints.
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
	}, s.patchProject)

	huma.Register(api, huma.Operation{
		OperationID: "projects-restore",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/projects/{id}/restore",
		Tags:        []string{"projects"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.restoreProject)
}

// requireProjectTeam rejects an unknown or archived team with 422, so a lead
// cannot create a project in a team's name that takes no work.
func (s *Server) requireProjectTeam(teamID uint) error {
	ok, err := s.Store.TeamAcceptsProjects(teamID)
	if err != nil {
		return internal(err)
	}
	if !ok {
		return problem(http.StatusUnprocessableEntity, urnInvalidInput, "team is archived or does not exist")
	}
	return nil
}

type projectListInput struct {
	All bool `query:"all" doc:"Include archived projects (admin only)."`
}

type projectListOutput struct {
	Body []projectResponse
}

func (s *Server) listProjects(ctx context.Context, in *projectListInput) (*projectListOutput, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	// Archived projects still count in reports, so only an admin sees them here.
	if in.All && !u.IsAdmin {
		return nil, problem(http.StatusForbidden, urnForbidden, "admin only")
	}
	projects, err := s.Store.Projects(in.All)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]projectResponse, 0, len(projects))
	for i := range projects {
		out = append(out, projectResponseOf(&projects[i]))
	}
	return &projectListOutput{Body: out}, nil
}

type projectCreateInput struct {
	Body struct {
		TeamID uint   `json:"team_id" required:"true" minimum:"1"`
		Name   string `json:"name" required:"true" minLength:"1"`
		Color  string `json:"color,omitempty"`
	}
}

type projectOutput struct {
	Body projectResponse
}

func (s *Server) createProject(ctx context.Context, in *projectCreateInput) (*projectOutput, error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}
	if err := s.requireProjectTeam(in.Body.TeamID); err != nil {
		return nil, err
	}
	u, err := s.requireTeamManager(ctx, in.Body.TeamID)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	p := &store.Project{TeamID: in.Body.TeamID, Name: in.Body.Name, Color: in.Body.Color, CreatedAt: now, UpdatedAt: now}
	err = s.Store.InTx(func(tx *store.Store) error {
		if err := tx.CreateProject(p); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, u), "project.create", "project", p.ID,
			store.Change{After: p.AuditFields()})
	})
	if err != nil {
		return nil, internal(err)
	}
	return &projectOutput{Body: projectResponseOf(p)}, nil
}

type projectPatchInput struct {
	ID   uint `path:"id"`
	Body struct {
		Name     *string `json:"name,omitempty" minLength:"1"`
		Color    *string `json:"color,omitempty"`
		TeamID   *uint   `json:"team_id,omitempty" minimum:"1"`
		Archived *bool   `json:"archived,omitempty"`
	}
}

func (s *Server) patchProject(ctx context.Context, in *projectPatchInput) (*projectOutput, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Store.ProjectByID(in.ID)
	if err != nil {
		return nil, fail(err)
	}
	ok, err := s.canManageTeam(u, p.TeamID)
	if err != nil {
		return nil, internal(err)
	}
	if !ok {
		return nil, problem(http.StatusForbidden, urnForbidden, "you do not lead this team")
	}
	body := in.Body
	teamID := p.TeamID
	if body.TeamID != nil && *body.TeamID != p.TeamID {
		if err := s.requireProjectTeam(*body.TeamID); err != nil {
			return nil, err
		}
		ok, err := s.canManageTeam(u, *body.TeamID)
		if err != nil {
			return nil, internal(err)
		}
		if !ok {
			return nil, problem(http.StatusForbidden, urnForbidden, "you do not lead the target team")
		}
		teamID = *body.TeamID
	}
	if body.Archived != nil {
		// Restoring is the admin's call, exactly as on POST /projects/{id}/restore.
		if !*body.Archived && !u.IsAdmin {
			return nil, problem(http.StatusForbidden, urnForbidden, "admin only")
		}
		// A project must not come back to life inside an archived team.
		if !*body.Archived {
			if err := s.requireProjectTeam(teamID); err != nil {
				return nil, err
			}
		}
	}
	now := s.Clock.Now()
	var after *store.Project
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.ProjectByID(in.ID)
		if err != nil {
			return err
		}
		p := *before
		if body.TeamID != nil && *body.TeamID != p.TeamID {
			// The check and the write are one statement, so an entry created in
			// between cannot land on a project that just changed teams.
			if err := tx.MoveProject(p.ID, *body.TeamID, now); err != nil {
				return err
			}
			p.TeamID = *body.TeamID
		}
		if body.Archived != nil {
			if *body.Archived {
				at := now
				p.ArchivedAt = &at
			} else {
				p.ArchivedAt = nil
			}
		}
		if body.Name != nil {
			p.Name = *body.Name
		}
		if body.Color != nil {
			p.Color = *body.Color
		}
		if err := tx.UpdateProject(&p); err != nil {
			return err
		}
		if after, err = tx.ProjectByID(in.ID); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, u), "project.update", "project", in.ID,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &projectOutput{Body: projectResponseOf(after)}, nil
}

type projectRestoreInput struct {
	ID uint `path:"id"`
}

func (s *Server) restoreProject(ctx context.Context, in *projectRestoreInput) (*projectOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Store.ProjectByID(in.ID)
	if err != nil {
		return nil, fail(err)
	}
	// A project must not come back to life inside an archived team.
	if err := s.requireProjectTeam(p.TeamID); err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	var after *store.Project
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.ProjectByID(in.ID)
		if err != nil {
			return err
		}
		p := *before
		p.ArchivedAt = nil
		if err := tx.UpdateProject(&p); err != nil {
			return err
		}
		if after, err = tx.ProjectByID(in.ID); err != nil {
			return err
		}
		return tx.RecordAudit(now, s.actorOf(ctx, admin), "project.restore", "project", in.ID,
			store.Change{Before: before.AuditFields(), After: after.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &projectOutput{Body: projectResponseOf(after)}, nil
}
