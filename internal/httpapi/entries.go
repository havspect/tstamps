package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/entry"
)

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

func toEntryResponse(e *entry.Entry) entryResponse {
	if e == nil {
		return entryResponse{}
	}
	return entryResponse{
		ID:        e.ID,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		Timezone:  e.Timezone,
		Note:      e.Note,
		DeletedAt: e.DeletedAt,
	}
}

type entryListInput struct {
	From           string `query:"from" doc:"Inclusive start date, YYYY-MM-DD. Defaults to the start of the caller's ISO week."`
	To             string `query:"to" doc:"Exclusive end date, YYYY-MM-DD. Required with from."`
	UserID         uint   `query:"user_id" doc:"Only this user's entries, within what the caller may read."`
	ProjectID      uint   `query:"project_id" doc:"Only this project's entries."`
	IncludeDeleted bool   `query:"include_deleted" doc:"Include soft-deleted entries, marked with deleted_at. Admin only; anyone else gets 403."`
}

type entryListOutput struct {
	Body []entryResponse
}

type entryCreateInput struct {
	Body struct {
		ProjectID uint      `json:"project_id" required:"true"`
		StartAt   time.Time `json:"start_at" required:"true"`
		EndAt     time.Time `json:"end_at" required:"true"`
		Timezone  string    `json:"timezone,omitempty"`
		Note      string    `json:"note,omitempty"`
	}
}

type entryCreateOutput struct {
	Body entryResponse
}

type entryPatchInput struct {
	ID   uint `path:"id"`
	Body struct {
		ProjectID *uint      `json:"project_id,omitempty"`
		StartAt   *time.Time `json:"start_at,omitempty"`
		EndAt     *time.Time `json:"end_at,omitempty"`
		Timezone  *string    `json:"timezone,omitempty"`
		Note      *string    `json:"note,omitempty"`
	}
}

type entryPatchOutput struct {
	Body entryResponse
}

type entryDeleteInput struct {
	ID uint `path:"id"`
}

type entryRestoreInput struct {
	ID uint `path:"id"`
}

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
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
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

func (s *Server) listEntries(ctx context.Context, in *entryListInput) (*entryListOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	filter := entry.ListFilter{
		UserID:         in.UserID,
		ProjectID:      in.ProjectID,
		IncludeDeleted: in.IncludeDeleted,
	}

	tz := "UTC"
	if s.deps.UserService != nil {
		if prof, err := s.deps.UserService.GetProfile(ctx); err == nil && prof != nil && prof.User.Timezone != "" {
			tz = prof.User.Timezone
		}
	}
	now := time.Now()
	if s.deps.Clock != nil {
		now = s.deps.Clock.Now()
	}

	if in.From == "" && in.To == "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
		local := now.In(loc)
		monday := time.Date(local.Year(), local.Month(), local.Day()-((int(local.Weekday())+6)%7), 0, 0, 0, 0, loc)
		y, m, d := monday.Date()
		ny, nm, nd := monday.AddDate(0, 0, 7).Date()
		f := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		t := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
		filter.From = &f
		filter.To = &t
	} else {
		if in.From == "" || in.To == "" {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "from and to must be given together")
		}
		f, err := time.Parse("2006-01-02", in.From)
		if err != nil {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid from date format")
		}
		t, err := time.Parse("2006-01-02", in.To)
		if err != nil {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid to date format")
		}
		if t.Before(f) {
			return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "to must be after from")
		}
		filter.From = &f
		filter.To = &t
	}

	entries, err := s.deps.EntryService.ListEntries(ctx, filter)
	if err != nil {
		return nil, fail(err)
	}

	out := make([]entryResponse, len(entries))
	for i, e := range entries {
		out[i] = toEntryResponse(&e)
	}
	return &entryListOutput{Body: out}, nil
}

func (s *Server) createEntry(ctx context.Context, in *entryCreateInput) (*entryCreateOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	tz := in.Body.Timezone
	if tz == "" && s.deps.UserService != nil {
		if prof, err := s.deps.UserService.GetProfile(ctx); err == nil && prof != nil && prof.User.Timezone != "" {
			tz = prof.User.Timezone
		}
	}

	e, err := s.deps.EntryService.LogEntry(ctx, in.Body.ProjectID, in.Body.StartAt, in.Body.EndAt, tz, in.Body.Note)
	if err != nil {
		return nil, fail(err)
	}

	return &entryCreateOutput{Body: toEntryResponse(e)}, nil
}

func (s *Server) patchEntry(ctx context.Context, in *entryPatchInput) (*entryPatchOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	e, err := s.deps.EntryService.PatchEntry(ctx, in.ID, in.Body.ProjectID, in.Body.StartAt, in.Body.EndAt, in.Body.Timezone, in.Body.Note)
	if err != nil {
		return nil, fail(err)
	}

	return &entryPatchOutput{Body: toEntryResponse(e)}, nil
}

func (s *Server) deleteEntry(ctx context.Context, in *entryDeleteInput) (*struct{}, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	err := s.deps.EntryService.DeleteEntry(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &struct{}{}, nil
}

func (s *Server) restoreEntry(ctx context.Context, in *entryRestoreInput) (*entryPatchOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	e, err := s.deps.EntryService.RestoreEntry(ctx, in.ID)
	if err != nil {
		return nil, fail(err)
	}

	return &entryPatchOutput{Body: toEntryResponse(e)}, nil
}
