// Package plan owns capacity planning: the forward-looking hours a person
// intends to spend on a project in one ISO week, held apart from entries
// (ADR-0022).
package plan

import (
	"errors"
	"math"
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
	// ErrInvalidVacation is returned for a vacation that ends before it starts,
	// is longer than a year, or has hours outside its working day.
	ErrInvalidVacation = errors.New("vacation must be a date range of at most 366 days with hours inside one working day")
	// ErrVacationOverlap is returned when a vacation covers a date another one
	// already covers.
	ErrVacationOverlap = errors.New("vacation overlaps another vacation")
)

const (
	// MaxHours is the most a person can be planned for in one week.
	MaxHours = 168
	// DefaultWeeklyHours is a person's capacity until an admin changes it.
	DefaultWeeklyHours = 40
	// MaxRepeatWeeks bounds one repeat call.
	MaxRepeatWeeks = 52
	// MaxVacationDays bounds one vacation range.
	MaxVacationDays = 366
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

// Vacation is a user's time off. Its dates are plain calendar dates in the
// user's timezone. StartHours and EndHours are the hours off on the first and
// last date (zero when that date is a weekend); interior working days take a
// full day. User and Email are filled when a list is read for display.
type Vacation struct {
	ID          uint
	WorkspaceID uint
	UserID      uint
	User        string
	Email       string
	StartDate   time.Time
	EndDate     time.Time
	StartHours  float64
	EndHours    float64
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

// PersonWeek is one person's capacity in one ISO week after vacation.
type PersonWeek struct {
	UserID        uint
	WeekStart     time.Time
	Capacity      int
	VacationHours float64
}

// Board is one range of planning data.
type Board struct {
	Weeks       []time.Time
	People      []Person
	Cells       []Cell
	Other       []OtherHours
	PersonWeeks []PersonWeek
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

// Date drops the time of day, keeping the calendar date in UTC.
func Date(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// IsWorkingDay reports whether t is Monday to Friday.
func IsWorkingDay(t time.Time) bool {
	weekday := t.Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

// DailyHours is one working day of a person's capacity. A person with no
// capacity set gets an eight-hour day so their own part-day entry still
// validates; their weekly capacity stays zero.
func DailyHours(weeklyHours int) float64 {
	if weeklyHours <= 0 {
		return 8
	}
	return float64(weeklyHours) / 5
}

// VacationHours counts the hours off in the Monday-to-Friday week that start one
// of the vacations. The first and last dates use their own hours when they are
// working days; the working days between them take a full day. Weekends never
// count, and a vacation outside the week contributes nothing.
func VacationHours(weekStart time.Time, weeklyHours int, vacations []Vacation) float64 {
	daily := DailyHours(weeklyHours)
	total := 0.0
	for offset := 0; offset < 5; offset++ {
		day := WeekStart(weekStart).AddDate(0, 0, offset)
		for _, v := range vacations {
			start, end := Date(v.StartDate), Date(v.EndDate)
			if day.Before(start) || day.After(end) {
				continue
			}
			switch {
			case start.Equal(end), day.Equal(start):
				total += v.StartHours
			case day.Equal(end):
				total += v.EndHours
			default:
				total += daily
			}
			break
		}
	}
	return total
}

// WeeklyCapacity is a person's hours for one week after vacation, rounded to the
// nearest hour and never below zero.
func WeeklyCapacity(weeklyHours int, vacationHours float64) int {
	if weeklyHours <= 0 {
		return 0
	}
	remaining := float64(weeklyHours) - vacationHours
	if remaining < 0 {
		remaining = 0
	}
	return int(math.Round(remaining))
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
