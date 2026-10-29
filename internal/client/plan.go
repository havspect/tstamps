package client

import (
	"net/http"
	"net/url"
)

// PlanQuery is the range and filters of a plan board. Empty dates default to
// next month on the server.
type PlanQuery struct {
	From   string
	To     string
	TeamID uint
	UserID uint
}

// PlanPerson is a person who can be planned, with their weekly capacity.
type PlanPerson struct {
	UserID      uint     `json:"user_id"`
	User        string   `json:"user"`
	Email       string   `json:"email"`
	WeeklyHours int      `json:"weekly_hours"`
	Teams       []string `json:"teams"`
	Deactivated bool     `json:"deactivated"`
}

// PlanCell is one person, project and ISO week of planned hours.
type PlanCell struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	Project   string `json:"project"`
	Archived  bool   `json:"archived"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

// PlanOther is a person's hours on projects the caller may not read.
type PlanOther struct {
	UserID    uint   `json:"user_id"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

// PlanBoard is one range of planning data.
type PlanBoard struct {
	Weeks  []string     `json:"weeks"`
	People []PlanPerson `json:"people"`
	Cells  []PlanCell   `json:"cells"`
	Other  []PlanOther  `json:"other"`
}

// PlanCellInput sets one cell. Hours 0 removes it.
type PlanCellInput struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

// SeedPlanInput fills a month from the previous month's logged hours.
type SeedPlanInput struct {
	FromMonth string `json:"from_month"`
	IntoMonth string `json:"into_month"`
	TeamID    uint   `json:"team_id,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// RepeatPlanInput copies one week's plan into the weeks after it.
type RepeatPlanInput struct {
	WeekStart string `json:"week_start"`
	Weeks     int    `json:"weeks"`
	TeamID    uint   `json:"team_id,omitempty"`
	UserID    uint   `json:"user_id,omitempty"`
	ProjectID uint   `json:"project_id,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// PlanSummaryCell is one cell a bulk wrote or would write.
type PlanSummaryCell struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	WeekStart string `json:"week_start"`
	Hours     int    `json:"hours"`
}

// PlanSummary reports what a seed or repeat did.
type PlanSummary struct {
	Filled  int               `json:"filled"`
	Skipped int               `json:"skipped"`
	Cells   []PlanSummaryCell `json:"cells"`
}

// Plan returns the board for a range.
func (c *Client) Plan(q PlanQuery) (*PlanBoard, error) {
	v := url.Values{}
	setString(v, "from", q.From)
	setString(v, "to", q.To)
	setUint(v, "team_id", q.TeamID)
	setUint(v, "user_id", q.UserID)
	var board PlanBoard
	if err := c.do(http.MethodGet, "/plan?"+v.Encode(), nil, &board); err != nil {
		return nil, err
	}
	return &board, nil
}

// SetPlan writes one cell; hours 0 removes it.
func (c *Client) SetPlan(in PlanCellInput) (*PlanCell, error) {
	var cell PlanCell
	if err := c.do(http.MethodPut, "/plan", in, &cell); err != nil {
		return nil, err
	}
	return &cell, nil
}

// SeedPlan fills a month from the previous month's logged hours.
func (c *Client) SeedPlan(in SeedPlanInput) (*PlanSummary, error) {
	var sum PlanSummary
	if err := c.do(http.MethodPost, "/plan/seed", in, &sum); err != nil {
		return nil, err
	}
	return &sum, nil
}

// RepeatPlan copies one week's plan into the weeks after it.
func (c *Client) RepeatPlan(in RepeatPlanInput) (*PlanSummary, error) {
	var sum PlanSummary
	if err := c.do(http.MethodPost, "/plan/repeat", in, &sum); err != nil {
		return nil, err
	}
	return &sum, nil
}
