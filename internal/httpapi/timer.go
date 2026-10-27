package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type timerStartInput struct {
	Body struct {
		ProjectID uint   `json:"project_id" required:"true"`
		Timezone  string `json:"timezone,omitempty"`
		Note      string `json:"note,omitempty"`
	}
}

type timerStartOutput struct {
	Body entryResponse
}

type timerStopInput struct {
	Body *struct {
		Note string `json:"note,omitempty"`
	}
}

type timerStopOutput struct {
	Body entryResponse
}

type timerGetOutput struct {
	Body *entryResponse
}

func (s *Server) timerRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "timer-get",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/timer",
		Tags:        []string{"timer"},
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError},
	}, s.getTimer)

	huma.Register(api, huma.Operation{
		OperationID:   "timer-start",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/timer/start",
		Tags:          []string{"timer"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.startTimer)

	huma.Register(api, huma.Operation{
		OperationID: "timer-stop",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/timer/stop",
		Tags:        []string{"timer"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.stopTimer)
}

func (s *Server) getTimer(ctx context.Context, _ *struct{}) (*timerGetOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	running, err := s.deps.EntryService.GetRunning(ctx)
	if err != nil {
		return nil, fail(err)
	}

	if running == nil {
		return &timerGetOutput{Body: nil}, nil
	}

	resp := toEntryResponse(running)
	return &timerGetOutput{Body: &resp}, nil
}

func (s *Server) startTimer(ctx context.Context, in *timerStartInput) (*timerStartOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	tz := in.Body.Timezone
	if tz == "" && s.deps.UserService != nil {
		if prof, err := s.deps.UserService.GetProfile(ctx); err == nil && prof != nil && prof.User.Timezone != "" {
			tz = prof.User.Timezone
		}
	}

	e, err := s.deps.EntryService.StartTimer(ctx, in.Body.ProjectID, tz, in.Body.Note)
	if err != nil {
		return nil, fail(err)
	}

	return &timerStartOutput{Body: toEntryResponse(e)}, nil
}

func (s *Server) stopTimer(ctx context.Context, in *timerStopInput) (*timerStopOutput, error) {
	if s.deps.EntryService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "entry service not available")
	}

	var note *string
	if in.Body != nil && in.Body.Note != "" {
		note = &in.Body.Note
	}
	e, err := s.deps.EntryService.StopTimer(ctx, note)
	if err != nil {
		return nil, fail(err)
	}

	return &timerStopOutput{Body: toEntryResponse(e)}, nil
}
