package plan

import (
	"context"
	"time"

	"github.com/havspect/tstamps/internal/report"
)

// Filter narrows the cells and people a board reads.
type Filter struct {
	From      time.Time
	To        time.Time
	UserIDs   []uint
	TeamID    uint
	UserID    uint
	ProjectID uint
}

// VacationFilter narrows vacations to one person and a date range. Zero values
// mean every person and every date.
type VacationFilter struct {
	UserID uint
	From   time.Time
	To     time.Time
}

// Repository persists plan cells and reads the people a board renders.
type Repository interface {
	List(ctx context.Context, filter Filter) ([]Cell, error)
	People(ctx context.Context) ([]Person, error)
	GetPerson(ctx context.Context, userID uint) (*Person, error)
	GetProject(ctx context.Context, id uint) (*Project, error)
	Get(ctx context.Context, userID, projectID uint, weekStart time.Time) (*Plan, error)
	Upsert(ctx context.Context, p *Plan) error
	Delete(ctx context.Context, userID, projectID uint, weekStart time.Time) error
	ListVacations(ctx context.Context, filter VacationFilter) ([]Vacation, error)
	CreateVacation(ctx context.Context, v *Vacation) error
	DeleteVacation(ctx context.Context, id, userID uint) error
}

// UnitOfWork runs a block inside one transaction.
type UnitOfWork interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditRecorder records one immutable audit event.
type AuditRecorder interface {
	Record(ctx context.Context, action, targetType string, targetID uint, changes any) error
}

// Project is the part of a project planning needs.
type Project struct {
	ID         uint
	TeamID     uint
	Name       string
	ArchivedAt *time.Time
}

// ReportSource generates the logged hours a seed starts from.
type ReportSource interface {
	GenerateReport(ctx context.Context, filter report.Filter) (*report.Report, error)
}
