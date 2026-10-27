package report

import (
	"context"
	"io"
	"time"

	"github.com/havspect/tstamps/internal/auth"
	"github.com/havspect/tstamps/internal/authz"
	"github.com/havspect/tstamps/internal/clock"
)

// Filter specifies report date boundaries, bucketing, and optional scoping.
type Filter struct {
	From      time.Time
	To        time.Time
	By        string // "day", "week", "month", "none" (default "week")
	TeamID    uint
	UserID    uint
	ProjectID uint
}

// Report holds the aggregated totals and rows resulting from a report generation.
type Report struct {
	Rows         []Row
	TotalSeconds int64
}

// ReportEntrySource provides the raw scoped data needed to compute report segments.
type ReportEntrySource interface {
	LoadReportData(ctx context.Context, filter Filter) (Input, error)
}

// Service generates aggregated reports and CSV exports.
type Service struct {
	source     ReportEntrySource
	authorizer authz.Authorizer
	clock      clock.Clock
}

// NewService creates a new report domain Service.
func NewService(source ReportEntrySource, authorizer authz.Authorizer, clk clock.Clock) *Service {
	return &Service{
		source:     source,
		authorizer: authorizer,
		clock:      clk,
	}
}

func (s *Service) checkAuth(ctx context.Context, filter Filter) error {
	if s.authorizer == nil {
		return nil
	}
	p, ok := auth.PrincipalFrom(ctx)
	if !ok || p == nil {
		return authz.ErrUnauthorized
	}
	var res any
	if filter.TeamID != 0 {
		res = authz.TeamResource{TeamID: filter.TeamID}
	} else if filter.ProjectID != 0 {
		res = authz.ProjectResource{TeamID: filter.TeamID}
	}
	return s.authorizer.Check(ctx, p, authz.ActionViewReport, res)
}

// GenerateReport compiles aggregated report rows for the requested filter.
func (s *Service) GenerateReport(ctx context.Context, filter Filter) (*Report, error) {
	if err := s.checkAuth(ctx, filter); err != nil {
		return nil, err
	}

	in, err := s.source.LoadReportData(ctx, filter)
	if err != nil {
		return nil, err
	}
	if in.Now.IsZero() && s.clock != nil {
		in.Now = s.clock.Now()
	}

	segments, err := Split(in, filter.From, filter.To)
	if err != nil {
		return nil, err
	}

	by := filter.By
	if by == "" {
		by = "week"
	}
	rows, total := Aggregate(segments, by)
	return &Report{
		Rows:         rows,
		TotalSeconds: total,
	}, nil
}

// ExportCSV writes the detailed per-segment CSV export to w.
func (s *Service) ExportCSV(ctx context.Context, filter Filter, w io.Writer) error {
	if err := s.checkAuth(ctx, filter); err != nil {
		return err
	}

	in, err := s.source.LoadReportData(ctx, filter)
	if err != nil {
		return err
	}
	if in.Now.IsZero() && s.clock != nil {
		in.Now = s.clock.Now()
	}

	segments, err := Split(in, filter.From, filter.To)
	if err != nil {
		return err
	}

	return WriteCSV(w, segments)
}
