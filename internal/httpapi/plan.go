package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/plan"
)

const dateLayout = "2006-01-02"

type planPersonWeekResponse struct {
	WeekStart     string  `json:"week_start"`
	Capacity      int     `json:"capacity"`
	VacationHours float64 `json:"vacation_hours"`
}

type planPersonResponse struct {
	UserID      uint                     `json:"user_id"`
	User        string                   `json:"user"`
	Email       string                   `json:"email"`
	WeeklyHours int                      `json:"weekly_hours"`
	Teams       []string                 `json:"teams"`
	TeamIDs     []uint                   `json:"team_ids"`
	Deactivated bool                     `json:"deactivated"`
	Weeks       []planPersonWeekResponse `json:"weeks"`
}

type planCellResponse struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	Project   string `json:"project"`
	Archived  bool   `json:"archived"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

type planOtherResponse struct {
	UserID    uint   `json:"user_id"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

type planBoardResponse struct {
	Weeks  []string             `json:"weeks"`
	People []planPersonResponse `json:"people"`
	Cells  []planCellResponse   `json:"cells"`
	Other  []planOtherResponse  `json:"other"`
}

type planListInput struct {
	From   string `query:"from" doc:"Inclusive start date, YYYY-MM-DD. Defaults to the first day of next month."`
	To     string `query:"to" doc:"Exclusive end date, YYYY-MM-DD. Defaults to the first day of the month after that."`
	TeamID uint   `query:"team_id" doc:"Only this team's people and projects."`
	UserID uint   `query:"user_id" doc:"Only this person's cells, within what the caller may read."`
}

type planListOutput struct {
	Body planBoardResponse
}

type planSetInput struct {
	Body struct {
		UserID    uint   `json:"user_id" required:"true"`
		ProjectID uint   `json:"project_id" required:"true"`
		WeekStart string `json:"week_start" required:"true" doc:"Any date in the ISO week, YYYY-MM-DD."`
		Hours     int    `json:"hours" required:"true" minimum:"0" maximum:"168" doc:"0 removes the cell."`
	}
}

type planSetOutput struct {
	Body struct {
		UserID    uint   `json:"user_id"`
		ProjectID uint   `json:"project_id"`
		WeekStart string `json:"week_start"`
		Hours     int    `json:"hours"`
	}
}

type planSeedInput struct {
	Body struct {
		FromMonth string `json:"from_month" required:"true" doc:"Month whose logged hours seed the plan, YYYY-MM."`
		IntoMonth string `json:"into_month" required:"true" doc:"Month that is planned, YYYY-MM."`
		TeamID    uint   `json:"team_id,omitempty"`
		DryRun    bool   `json:"dry_run,omitempty" doc:"Report what would be filled without writing."`
	}
}

type planRepeatInput struct {
	Body struct {
		WeekStart string `json:"week_start" required:"true" doc:"Any date in the source ISO week, YYYY-MM-DD."`
		Weeks     int    `json:"weeks" required:"true" minimum:"1" maximum:"52"`
		TeamID    uint   `json:"team_id,omitempty"`
		UserID    uint   `json:"user_id,omitempty"`
		ProjectID uint   `json:"project_id,omitempty"`
		DryRun    bool   `json:"dry_run,omitempty"`
	}
}

type planSummaryCell struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

type planSummaryResponse struct {
	Filled  int               `json:"filled"`
	Skipped int               `json:"skipped"`
	Cells   []planSummaryCell `json:"cells"`
}

type planSummaryOutput struct {
	Body planSummaryResponse
}

func (s *Server) planRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "plan-get",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/plan",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.getPlan)

	huma.Register(api, huma.Operation{
		OperationID: "plan-set",
		Method:      http.MethodPut,
		Path:        apiPrefix + "/plan",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.setPlan)

	huma.Register(api, huma.Operation{
		OperationID: "plan-seed",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/plan/seed",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.seedPlan)

	huma.Register(api, huma.Operation{
		OperationID: "plan-repeat",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/plan/repeat",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.repeatPlan)
}

// nextMonth is the default board: the month that has not happened yet.
func nextMonth(now time.Time, tz string) (time.Time, time.Time) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func (s *Server) planRange(ctx context.Context, from, to string) (time.Time, time.Time, error) {
	if from == "" && to == "" {
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
		f, t := nextMonth(now, tz)
		return f, t, nil
	}
	if from == "" || to == "" {
		return time.Time{}, time.Time{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "from and to must be given together")
	}
	f, err := time.Parse(dateLayout, from)
	if err != nil {
		return time.Time{}, time.Time{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid from date format")
	}
	t, err := time.Parse(dateLayout, to)
	if err != nil {
		return time.Time{}, time.Time{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid to date format")
	}
	if !t.After(f) {
		return time.Time{}, time.Time{}, problem(http.StatusUnprocessableEntity, urnInvalidInput, "to must be after from")
	}
	return f, t, nil
}

func (s *Server) getPlan(ctx context.Context, in *planListInput) (*planListOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	from, to, err := s.planRange(ctx, in.From, in.To)
	if err != nil {
		return nil, err
	}
	board, err := s.deps.PlanService.List(ctx, plan.Filter{
		From: from, To: to, TeamID: in.TeamID, UserID: in.UserID,
	})
	if err != nil {
		return nil, fail(err)
	}

	out := &planListOutput{}
	out.Body.Weeks = make([]string, len(board.Weeks))
	for i, week := range board.Weeks {
		out.Body.Weeks[i] = week.Format(dateLayout)
	}
	out.Body.People = make([]planPersonResponse, len(board.People))
	weeksByUser := map[uint][]planPersonWeekResponse{}
	for _, pw := range board.PersonWeeks {
		weeksByUser[pw.UserID] = append(weeksByUser[pw.UserID], planPersonWeekResponse{
			WeekStart: pw.WeekStart.Format(dateLayout), Capacity: pw.Capacity, VacationHours: pw.VacationHours,
		})
	}
	for i, person := range board.People {
		teams := make([]string, len(person.Teams))
		teamIDs := make([]uint, len(person.Teams))
		for j, team := range person.Teams {
			teams[j] = team.Name
			teamIDs[j] = team.ID
		}
		out.Body.People[i] = planPersonResponse{
			UserID: person.UserID, User: person.Name, Email: person.Email,
			WeeklyHours: person.WeeklyHours, Teams: teams, TeamIDs: teamIDs, Deactivated: !person.Active,
			Weeks: weeksByUser[person.UserID],
		}
	}
	out.Body.Cells = make([]planCellResponse, len(board.Cells))
	for i, cell := range board.Cells {
		out.Body.Cells[i] = planCellResponse{
			UserID: cell.UserID, ProjectID: cell.ProjectID, Project: cell.Project,
			Archived: cell.Archived, WeekStart: cell.WeekStart.Format(dateLayout), Hours: cell.Hours,
		}
	}
	out.Body.Other = make([]planOtherResponse, len(board.Other))
	for i, other := range board.Other {
		out.Body.Other[i] = planOtherResponse{
			UserID: other.UserID, WeekStart: other.WeekStart.Format(dateLayout), Hours: other.Hours,
		}
	}
	return out, nil
}

func (s *Server) setPlan(ctx context.Context, in *planSetInput) (*planSetOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	week, err := time.Parse(dateLayout, in.Body.WeekStart)
	if err != nil {
		return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid week_start date format")
	}
	cell, err := s.deps.PlanService.Set(ctx, in.Body.UserID, in.Body.ProjectID, week, in.Body.Hours)
	if err != nil {
		return nil, fail(err)
	}
	out := &planSetOutput{}
	out.Body.UserID = cell.UserID
	out.Body.ProjectID = cell.ProjectID
	out.Body.WeekStart = cell.WeekStart.Format(dateLayout)
	out.Body.Hours = cell.Hours
	return out, nil
}

func (s *Server) seedPlan(ctx context.Context, in *planSeedInput) (*planSummaryOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	sum, err := s.deps.PlanService.Seed(ctx, plan.SeedInput{
		FromMonth: in.Body.FromMonth, IntoMonth: in.Body.IntoMonth,
		TeamID: in.Body.TeamID, DryRun: in.Body.DryRun,
	})
	if err != nil {
		return nil, fail(err)
	}
	return &planSummaryOutput{Body: toPlanSummary(sum)}, nil
}

func (s *Server) repeatPlan(ctx context.Context, in *planRepeatInput) (*planSummaryOutput, error) {
	if s.deps.PlanService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "plan service not available")
	}
	week, err := time.Parse(dateLayout, in.Body.WeekStart)
	if err != nil {
		return nil, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid week_start date format")
	}
	sum, err := s.deps.PlanService.Repeat(ctx, plan.RepeatInput{
		WeekStart: week, Weeks: in.Body.Weeks, TeamID: in.Body.TeamID,
		UserID: in.Body.UserID, ProjectID: in.Body.ProjectID, DryRun: in.Body.DryRun,
	})
	if err != nil {
		return nil, fail(err)
	}
	return &planSummaryOutput{Body: toPlanSummary(sum)}, nil
}

func toPlanSummary(sum *plan.Summary) planSummaryResponse {
	out := planSummaryResponse{Filled: sum.Filled, Skipped: sum.Skipped}
	out.Cells = make([]planSummaryCell, len(sum.Cells))
	for i, cell := range sum.Cells {
		out.Cells[i] = planSummaryCell{
			UserID: cell.UserID, ProjectID: cell.ProjectID,
			WeekStart: cell.WeekStart.Format(dateLayout), Hours: cell.Hours,
		}
	}
	return out
}
