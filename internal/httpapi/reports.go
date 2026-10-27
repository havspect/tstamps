package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/havspect/tstamps/internal/report"
)

type reportRow struct {
	Bucket    string `json:"bucket"`
	UserID    uint   `json:"user_id"`
	User      string `json:"user"`
	Email     string `json:"email"`
	TeamID    uint   `json:"team_id"`
	Team      string `json:"team"`
	ProjectID uint   `json:"project_id"`
	Project   string `json:"project"`
	Archived  bool   `json:"archived"`
	Seconds   int64  `json:"seconds"`
}

type reportInput struct {
	From      string `query:"from" doc:"Inclusive start date, YYYY-MM-DD. Defaults to the start of the caller's ISO week."`
	To        string `query:"to" doc:"Exclusive end date, YYYY-MM-DD. Required with from."`
	By        string `query:"by" enum:"week,day,month,none" default:"week" doc:"Bucket: ISO week, entry-local day, entry-local month, or the whole range."`
	TeamID    uint   `query:"team_id" doc:"Only entries on this team's projects."`
	UserID    uint   `query:"user_id" doc:"Only this user's entries, within what the caller may read."`
	ProjectID uint   `query:"project_id" doc:"Only this project's entries."`
}

type reportOutput struct {
	Body struct {
		Rows         []reportRow `json:"rows"`
		TotalSeconds int64       `json:"total_seconds"`
	}
}

type reportCSVOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

func (s *Server) reportRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "reports-get",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/reports",
		Tags:        []string{"reports"},
		Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, s.getReport)

	huma.Register(api, huma.Operation{
		OperationID: "reports-csv",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/reports.csv",
		Tags:        []string{"reports"},
		Errors:      []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "CSV export: one row per entry per local day.",
				Content: map[string]*huma.MediaType{
					"text/csv": {Schema: &huma.Schema{Type: "string"}},
				},
			},
		},
	}, s.getReportCSV)
}

func (s *Server) buildReportFilter(ctx context.Context, in *reportInput) (report.Filter, error) {
	by := in.By
	if by == "" {
		by = "week"
	}

	filter := report.Filter{
		By:        by,
		TeamID:    in.TeamID,
		UserID:    in.UserID,
		ProjectID: in.ProjectID,
	}

	if in.From != "" || in.To != "" {
		if in.From == "" || in.To == "" {
			return filter, problem(http.StatusUnprocessableEntity, urnInvalidInput, "from and to must be given together")
		}
		f, err := time.Parse("2006-01-02", in.From)
		if err != nil {
			return filter, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid from date format")
		}
		t, err := time.Parse("2006-01-02", in.To)
		if err != nil {
			return filter, problem(http.StatusUnprocessableEntity, urnInvalidInput, "invalid to date format")
		}
		if t.Before(f) {
			return filter, problem(http.StatusUnprocessableEntity, urnInvalidInput, "to must be after from")
		}
		if t.Sub(f) > 366*24*time.Hour {
			return filter, problem(http.StatusUnprocessableEntity, urnInvalidInput, "the range may not be longer than 366 days")
		}
		filter.From = f
		filter.To = t
	} else {
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
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
		local := now.In(loc)
		monday := time.Date(local.Year(), local.Month(), local.Day()-((int(local.Weekday())+6)%7), 0, 0, 0, 0, loc)
		y, m, d := monday.Date()
		ny, nm, nd := monday.AddDate(0, 0, 7).Date()
		filter.From = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		filter.To = time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
	}

	return filter, nil
}

func (s *Server) getReport(ctx context.Context, in *reportInput) (*reportOutput, error) {
	if s.deps.ReportService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "report service not available")
	}

	filter, err := s.buildReportFilter(ctx, in)
	if err != nil {
		return nil, err
	}

	rep, err := s.deps.ReportService.GenerateReport(ctx, filter)
	if err != nil {
		return nil, fail(err)
	}

	out := &reportOutput{}
	out.Body.Rows = make([]reportRow, len(rep.Rows))
	for i, r := range rep.Rows {
		out.Body.Rows[i] = reportRow{
			Bucket:    r.Bucket,
			UserID:    r.UserID,
			User:      r.User,
			Email:     r.Email,
			TeamID:    r.TeamID,
			Team:      r.Team,
			ProjectID: r.ProjectID,
			Project:   r.Project,
			Archived:  r.Archived,
			Seconds:   r.Seconds,
		}
	}
	out.Body.TotalSeconds = rep.TotalSeconds
	return out, nil
}

func (s *Server) getReportCSV(ctx context.Context, in *reportInput) (*reportCSVOutput, error) {
	if s.deps.ReportService == nil {
		return nil, problem(http.StatusServiceUnavailable, urnUnavailable, "report service not available")
	}

	filter, err := s.buildReportFilter(ctx, in)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := s.deps.ReportService.ExportCSV(ctx, filter, &buf); err != nil {
		return nil, fail(err)
	}

	return &reportCSVOutput{
		ContentType: "text/csv; charset=utf-8",
		Body:        buf.Bytes(),
	}, nil
}
