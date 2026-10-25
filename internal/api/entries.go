package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/store"
)

// entryResponse is an entry as every entry and timer reply shows it. Project and
// user names are deliberately absent: the client already has them, and copying
// them here would go stale.
type entryResponse struct {
	ID        uint       `json:"id"`
	UserID    uint       `json:"user_id"`
	ProjectID uint       `json:"project_id"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at"`
	Timezone  string     `json:"timezone"`
	Note      string     `json:"note"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

func entryResponseOf(e *store.Entry) entryResponse {
	return entryResponse{
		ID: e.ID, UserID: e.UserID, ProjectID: e.ProjectID,
		StartAt: e.StartAt, EndAt: e.EndAt, Timezone: e.Timezone, Note: e.Note, DeletedAt: e.DeletedAt,
	}
}

// entryRoutes registers the entry endpoints.
func (s *Server) entryRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "entries-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/entries",
		Tags:        []string{"entries"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.listEntries)

	huma.Register(api, huma.Operation{
		OperationID:   "entries-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/entries",
		Tags:          []string{"entries"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createEntry)

	huma.Register(api, huma.Operation{
		OperationID: "entries-update",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/entries/{id}",
		Tags:        []string{"entries"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.patchEntry)

	huma.Register(api, huma.Operation{
		OperationID:   "entries-delete",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/entries/{id}",
		Tags:          []string{"entries"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.deleteEntry)

	huma.Register(api, huma.Operation{
		OperationID: "entries-restore",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/entries/{id}/restore",
		Tags:        []string{"entries"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.restoreEntry)
}

type entryListInput struct {
	From           string `query:"from" doc:"Inclusive start date, YYYY-MM-DD. Defaults to the start of the caller's ISO week."`
	To             string `query:"to" doc:"Exclusive end date, YYYY-MM-DD. Required with from."`
	UserID         uint   `query:"user_id" doc:"Only this user's entries, within what the caller may read."`
	ProjectID      uint   `query:"project_id" doc:"Only this project's entries."`
	IncludeDeleted bool   `query:"include_deleted" doc:"Include soft-deleted entries, marked with deleted_at. Admin only; anyone else gets 403."`
}

type entryListOutput struct{ Body []entryResponse }

func (s *Server) listEntries(ctx context.Context, in *entryListInput) (*entryListOutput, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if in.IncludeDeleted {
		if _, err := requireAdmin(ctx); err != nil {
			return nil, err
		}
	}
	scope, err := s.entryScope(u)
	if err != nil {
		return nil, internal(err)
	}
	scope.IncludeDeleted = in.IncludeDeleted
	rng, err := parseRange(u.Timezone, s.Clock.Now(), in.From, in.To)
	if err != nil {
		return nil, err
	}
	page, err := s.readEntries(scope, rng, entryFilter{UserID: in.UserID, ProjectID: in.ProjectID})
	if err != nil {
		return nil, internal(err)
	}
	out := make([]entryResponse, 0, len(page.entries))
	for i := range page.entries {
		out = append(out, entryResponseOf(&page.entries[i]))
	}
	return &entryListOutput{Body: out}, nil
}

type entryCreateInput struct {
	Body struct {
		ProjectID uint      `json:"project_id" required:"true" minimum:"1"`
		StartAt   time.Time `json:"start_at" required:"true"`
		EndAt     time.Time `json:"end_at" required:"true"`
		Timezone  string    `json:"timezone" required:"true" minLength:"1" doc:"IANA zone the entry was logged in, e.g. Europe/Berlin."`
		Note      string    `json:"note,omitempty"`
	}
}

type entryOutput struct{ Body entryResponse }

func (s *Server) createEntry(ctx context.Context, in *entryCreateInput) (*entryOutput, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateTimezone(in.Body.Timezone); err != nil {
		return nil, err
	}
	e, err := s.entries.Create(u.ID, in.Body.ProjectID, in.Body.StartAt, in.Body.EndAt, in.Body.Timezone, in.Body.Note)
	if err != nil {
		return nil, fail(err)
	}
	return &entryOutput{Body: entryResponseOf(e)}, nil
}

type entryPatchInput struct {
	ID   uint `path:"id"`
	Body struct {
		ProjectID *uint      `json:"project_id,omitempty" minimum:"1"`
		StartAt   *time.Time `json:"start_at,omitempty"`
		EndAt     *time.Time `json:"end_at,omitempty"`
		Timezone  *string    `json:"timezone,omitempty" minLength:"1" doc:"Correct the IANA zone the entry was logged in."`
		Note      *string    `json:"note,omitempty"`
	}
}

func (s *Server) patchEntry(ctx context.Context, in *entryPatchInput) (*entryOutput, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if in.Body.Timezone != nil {
		if err := validateTimezone(*in.Body.Timezone); err != nil {
			return nil, err
		}
	}
	e, err := s.entries.Patch(u.ID, in.ID, entry.Patch{
		ProjectID: in.Body.ProjectID, StartAt: in.Body.StartAt,
		EndAt: in.Body.EndAt, Timezone: in.Body.Timezone, Note: in.Body.Note,
	})
	if err != nil {
		return nil, fail(err)
	}
	return &entryOutput{Body: entryResponseOf(e)}, nil
}

type entryDeleteInput struct {
	ID uint `path:"id"`
}

func (s *Server) deleteEntry(ctx context.Context, in *entryDeleteInput) (*struct{}, error) {
	u, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.entries.Delete(u.ID, in.ID); err != nil {
		return nil, fail(err)
	}
	return &struct{}{}, nil
}

type entryRestoreInput struct {
	ID uint `path:"id"`
}

func (s *Server) restoreEntry(ctx context.Context, in *entryRestoreInput) (*entryOutput, error) {
	admin, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	var restored *store.Entry
	var at time.Time
	err = s.Store.InTx(func(tx *store.Store) error {
		before, err := tx.EntryByID(in.ID)
		if err != nil {
			return err
		}
		// The shared service is bound to the non-transactional store, so use one
		// bound to tx instead.
		restored, err = entry.New(tx, s.Clock).Restore(in.ID)
		if err != nil {
			return err
		}
		// Restore takes its own clock reading; its updated_at is that exact
		// instant, so the event and the row carry the same time.
		at = restored.UpdatedAt
		return tx.RecordAudit(at, s.actorOf(ctx, admin), "entry.restore", "entry", in.ID,
			store.Change{Before: before.AuditFields(), After: restored.AuditFields()})
	})
	if err != nil {
		return nil, fail(err)
	}
	return &entryOutput{Body: entryResponseOf(restored)}, nil
}
