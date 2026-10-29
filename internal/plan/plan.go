// Package plan owns capacity planning: the forward-looking hours a person
// intends to spend on a project in one ISO week, held apart from entries
// (ADR-0022).
package plan

import (
	"errors"
	"time"
)

var (
	// ErrInvalidHours is returned when a cell is outside 0–168 hours.
	ErrInvalidHours = errors.New("plan hours must be between 0 and 168")
	// ErrFrozenWeek is returned when a write targets a week that has ended.
	ErrFrozenWeek = errors.New("the week has ended")
	// ErrNotFound is returned when the person or the project does not exist.
	ErrNotFound = errors.New("not found")
	// ErrProjectNotPlannable is returned when a project is archived or unknown.
	ErrProjectNotPlannable = errors.New("project cannot be planned")
	// ErrUnknownMonth is returned for a month that is not YYYY-MM.
	ErrUnknownMonth = errors.New("month must be YYYY-MM")
	// ErrInvalidRange is returned when a board range is empty or backwards.
	ErrInvalidRange = errors.New("from and to must form a range")
)

const (
	// MaxHours is the most a person can be planned for in one week.
	MaxHours = 168
	// DefaultWeeklyHours is a person's capacity until an admin changes it.
	DefaultWeeklyHours = 40
	// MaxRepeatWeeks bounds one repeat call.
	MaxRepeatWeeks = 52
)

// Plan is one cell: a person, a project, an ISO week and a number of hours.
// WeekStart is the Monday of that week, at midnight UTC, date only.
type Plan struct {
	ID          uint
	WorkspaceID uint
	UserID      uint
	ProjectID   uint
	WeekStart   time.Time
	Hours       int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AuditFields returns the cell as a map for the audit log.
func (p *Plan) AuditFields() map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"id":           p.ID,
		"workspace_id": p.WorkspaceID,
		"user_id":      p.UserID,
		"project_id":   p.ProjectID,
		"week_start":   p.WeekStart.Format("2006-01-02"),
		"hours":        p.Hours,
	}
}

// TeamRef names one team a person belongs to.
type TeamRef struct {
	ID   uint
	Name string
}

// Person is a user who can be planned, with the capacity of their week.
type Person struct {
	UserID      uint
	Name        string
	Email       string
	Timezone    string
	WeeklyHours int
	Active      bool
	Teams       []TeamRef
}

// Cell is a stored plan with the names and team it needs to render.
type Cell struct {
	UserID    uint
	User      string
	Email     string
	ProjectID uint
	Project   string
	TeamID    uint
	Team      string
	Archived  bool
	WeekStart time.Time
	Hours     int
}

// OtherHours is a person's planned hours on projects the caller may not read.
type OtherHours struct {
	UserID    uint
	WeekStart time.Time
	Hours     int
}

// Board is one range of planning data.
type Board struct {
	Weeks  []time.Time
	People []Person
	Cells  []Cell
	Other  []OtherHours
}

// Summary reports what a bulk write did.
type Summary struct {
	Filled  int
	Skipped int
	Cells   []Cell
}

// WeekStart returns the Monday of t's ISO week, as a UTC date.
func WeekStart(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
}

// WeeksIn lists the Mondays in [from, to). A week belongs to the month its
// Monday falls in, so a week straddling a month boundary is not counted twice.
func WeeksIn(from, to time.Time) []time.Time {
	w := WeekStart(from)
	if w.Before(from) {
		w = w.AddDate(0, 0, 7)
	}
	var weeks []time.Time
	for ; w.Before(to); w = w.AddDate(0, 0, 7) {
		weeks = append(weeks, w)
	}
	return weeks
}

// MonthStart parses YYYY-MM into its first day and the first day of the next month.
func MonthStart(month string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, time.Time{}, ErrUnknownMonth
	}
	return start, start.AddDate(0, 1, 0), nil
}

// Ended reports whether the week that starts at weekStart has passed for a
// person in tz: from the following Monday at midnight it is history.
func Ended(weekStart time.Time, tz string, now time.Time) bool {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	end := weekStart.AddDate(0, 0, 7)
	monday := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
	return !now.Before(monday)
}

// ValidHours reports whether h can be stored.
func ValidHours(h int) bool { return h >= 0 && h <= MaxHours }

// RoundHours converts seconds to whole hours, rounding to the nearest hour and
// never below one for a non-zero input.
func RoundHours(seconds int64) int {
	if seconds <= 0 {
		return 0
	}
	h := int((float64(seconds)/3600 + 0.5))
	if h < 1 {
		h = 1
	}
	return h
}
