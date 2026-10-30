package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/plan"
)

type vacationResponse struct {
	ID         uint    `json:"id"`
	UserID     uint    `json:"user_id"`
	User       string  `json:"user"`
	Email      string  `json:"email"`
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	StartHours float64 `json:"start_hours"`
	EndHours   float64 `json:"end_hours"`
	Note       string  `json:"note"`
}

func toVacationResponse(v *plan.Vacation) vacationResponse {
	return vacationResponse{
		ID:         v.ID,
		UserID:     v.UserID,
		User:       v.User,
		Email:      v.Email,
		StartDate:  v.StartDate.Format(dateLayout),
		EndDate:    v.EndDate.Format(dateLayout),
		StartHours: v.StartHours,
		EndHours:   v.EndHours,
		Note:       v.Note,
	}
}

type vacationListInput struct {
	UserID uint `query:"user_id" doc:"Whose vacations to read. Defaults to the caller; a lead may read their teams, an admin anyone."`
}

type vacationListOutput struct {
	Body []vacationResponse
}

type vacationCreateInput struct {
	Body struct {
		StartDate  string  `json:"start_date" required:"true" doc:"First day off, YYYY-MM-DD."`
		EndDate    string  `json:"end_date" required:"true" doc:"Last day off, inclusive, YYYY-MM-DD."`
		StartHours float64 `json:"start_hours" required:"true" minimum:"0" doc:"Hours off on the first date, at most one working day. Zero on a weekend."`
		EndHours   float64 `json:"end_hours,omitempty" minimum:"0" doc:"Hours off on the last date. Omit or send 0 for a one-day vacation or a weekend."`
		Note       string  `json:"note,omitempty"`
	}
}

type vacationCreateOutput struct {
	Body vacationResponse
}

type vacationDeleteInput struct {
	ID uint `path:"id"`
}

func (s *Server) vacationRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "vacations-list",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/vacations",
		Tags:        []string{"vacations"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.listVacations)

	huma.Register(api, huma.Operation{
		OperationID:   "vacations-create",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/vacations",
		Tags:          []string{"vacations"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.createVacation)

	huma.Register(api, huma.Operation{
		OperationID:   "vacations-delete",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/vacations/{id}",
		Tags:          []string{"vacations"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
	}, s.deleteVacation)
}

func (s *Server) listVacations(ctx context.Context, in *vacationListInput) (*vacationListOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	vacations, err := s.deps.PlanService.ListVacations(ctx, plan.VacationQuery{UserID: in.UserID})
	if err != nil {
		return nil, fail(err)
	}
	out := make([]vacationResponse, len(vacations))
	for i, vacation := range vacations {
		out[i] = toVacationResponse(&vacation)
	}
	return &vacationListOutput{Body: out}, nil
}

func (s *Server) createVacation(ctx context.Context, in *vacationCreateInput) (*vacationCreateOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	start, err := time.Parse(dateLayout, in.Body.StartDate)
	if err != nil {
		return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid start_date date format")
	}
	end, err := time.Parse(dateLayout, in.Body.EndDate)
	if err != nil {
		return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid end_date date format")
	}
	vacation, err := s.deps.PlanService.AddVacation(ctx, plan.VacationInput{
		StartDate: start, EndDate: end,
		StartHours: in.Body.StartHours, EndHours: in.Body.EndHours,
		Note: in.Body.Note,
	})
	if err != nil {
		return nil, fail(err)
	}
	return &vacationCreateOutput{Body: toVacationResponse(vacation)}, nil
}

func (s *Server) deleteVacation(ctx context.Context, in *vacationDeleteInput) (*struct{}, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	if err := s.deps.PlanService.DeleteVacation(ctx, in.ID); err != nil {
		return nil, fail(err)
	}
	return &struct{}{}, nil
}
