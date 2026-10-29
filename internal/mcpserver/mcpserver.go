// Package mcpserver exposes the Tstamps MCP tool catalogue, calling domain feature
// services directly as an inbound transport ("One Way In").
package mcpserver

import (
	"context"
	"net/http"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/plan"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
	"github.com/havspect/tstamps/internal/version"
)

// UserService defines the user domain operations needed by MCP tools.
type UserService interface {
	GetProfile(ctx context.Context) (*user.Profile, error)
	ListUsers(ctx context.Context) ([]user.User, error)
}

// ProjectService defines the project domain operations needed by MCP tools.
type ProjectService interface {
	ListProjects(ctx context.Context, includeArchived bool) ([]project.Project, error)
}

// TeamService defines the team domain operations needed by MCP tools.
type TeamService interface {
	ListTeams(ctx context.Context, includeArchived bool) ([]team.TeamWithMembers, error)
}

// EntryService defines the entry domain operations needed by MCP tools.
type EntryService interface {
	ListEntries(ctx context.Context, filter entry.ListFilter) ([]entry.Entry, error)
	StartTimer(ctx context.Context, projectID uint, tz, note string) (*entry.Entry, error)
	StopTimer(ctx context.Context, note *string) (*entry.Entry, error)
	LogEntry(ctx context.Context, projectID uint, start, end time.Time, tz, note string) (*entry.Entry, error)
	PatchEntry(ctx context.Context, id uint, projectID *uint, start, end *time.Time, timezone *string, note *string) (*entry.Entry, error)
	DeleteEntry(ctx context.Context, id uint) error
}

// ReportService defines the report domain operations needed by MCP tools.
type ReportService interface {
	GenerateReport(ctx context.Context, filter report.Filter) (*report.Report, error)
}

// PlanService defines the plan domain operations needed by MCP tools.
type PlanService interface {
	List(ctx context.Context, filter plan.Filter) (*plan.Board, error)
	Set(ctx context.Context, userID, projectID uint, weekStart time.Time, hours int) (*plan.Plan, error)
	Seed(ctx context.Context, in plan.SeedInput) (*plan.Summary, error)
	Repeat(ctx context.Context, in plan.RepeatInput) (*plan.Summary, error)
}

// Config configures the MCP server tool catalogue with domain services.
type Config struct {
	UserService    UserService
	ProjectService ProjectService
	TeamService    TeamService
	EntryService   EntryService
	ReportService  ReportService
	PlanService    PlanService
}

// NewServer returns an MCP server with all tools registered directly to domain services.
func NewServer(cfg Config) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{
		Name:    "tstamps",
		Title:   "Tstamps",
		Version: version.Version,
	}, nil)
	register(s, cfg)
	return s
}

// NewHandler returns an HTTP handler for streamable MCP over HTTP.
func NewHandler(cfg Config) http.Handler {
	return sdk.NewStreamableHTTPHandler(func(req *http.Request) *sdk.Server {
		return NewServer(cfg)
	}, &sdk.StreamableHTTPOptions{Stateless: true})
}

// ServeStdio runs the server over stdin and stdout until the client closes the pipe.
func ServeStdio(ctx context.Context, cfg Config) error {
	return NewServer(cfg).Run(ctx, &sdk.StdioTransport{})
}

func readOnly(name, description string) *sdk.Tool {
	return &sdk.Tool{
		Name:        name,
		Description: description,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}
}

func write(name, description string) *sdk.Tool {
	return &sdk.Tool{
		Name:        name,
		Description: description,
	}
}

func toEntryDTO(e *entry.Entry) Entry {
	if e == nil {
		return Entry{}
	}
	return Entry{
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

func parseDateRange(fromStr, toStr, tz string, now time.Time) (time.Time, time.Time, error) {
	if fromStr == "" && toStr == "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
		local := now.In(loc)
		monday := time.Date(local.Year(), local.Month(), local.Day()-((int(local.Weekday())+6)%7), 0, 0, 0, 0, loc)
		y, m, d := monday.Date()
		ny, nm, nd := monday.AddDate(0, 0, 7).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC), nil
	}
	f, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	t, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return f, t, nil
}

func register(s *sdk.Server, cfg Config) {
	sdk.AddTool(s, readOnly("whoami",
		"Return the authenticated user: id, email, name, timezone, admin flag and team roles. Needs only a valid token."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, Me, error) {
			if cfg.UserService == nil {
				return nil, Me{}, nil
			}
			prof, err := cfg.UserService.GetProfile(ctx)
			if err != nil {
				return nil, Me{}, err
			}
			memberships := make([]Membership, 0, len(prof.LedTeamIDs))
			for _, tid := range prof.LedTeamIDs {
				memberships = append(memberships, Membership{TeamID: tid, Role: "lead"})
			}
			return nil, Me{
				ID:                 prof.User.ID,
				Email:              prof.User.Email,
				Name:               prof.User.Name,
				Timezone:           prof.User.Timezone,
				IsAdmin:            prof.User.IsAdmin,
				MustChangePassword: prof.User.MustChangePassword,
				Memberships:        memberships,
			}, nil
		})

	sdk.AddTool(s, readOnly("list_projects",
		"List projects to log against, with their team and archive state. Any member may list non-archived projects; all=true includes archived projects and needs an admin."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ListProjectsInput) (*sdk.CallToolResult, []Project, error) {
			if cfg.ProjectService == nil {
				return nil, []Project{}, nil
			}
			projs, err := cfg.ProjectService.ListProjects(ctx, in.All)
			if err != nil {
				return nil, nil, err
			}
			out := make([]Project, len(projs))
			for i, p := range projs {
				out[i] = Project{
					ID:         p.ID,
					TeamID:     p.TeamID,
					Name:       p.Name,
					Color:      p.Color,
					ArchivedAt: p.ArchivedAt,
				}
			}
			return nil, out, nil
		})

	sdk.AddTool(s, readOnly("list_teams",
		"List every team with its members and roles. Admin only."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []Team, error) {
			if cfg.TeamService == nil {
				return nil, []Team{}, nil
			}
			teams, err := cfg.TeamService.ListTeams(ctx, false)
			if err != nil {
				return nil, nil, err
			}
			out := make([]Team, len(teams))
			for i, t := range teams {
				members := make([]TeamMember, len(t.Members))
				for j, m := range t.Members {
					members[j] = TeamMember{
						UserID: m.UserID,
						Role:   m.Role,
					}
				}
				out[i] = Team{
					ID:         t.Team.ID,
					Name:       t.Team.Name,
					ArchivedAt: t.Team.ArchivedAt,
					Members:    members,
				}
			}
			return nil, out, nil
		})

	sdk.AddTool(s, readOnly("list_users",
		"List every user with timezone, admin flag and active state. Admin only."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []User, error) {
			if cfg.UserService == nil {
				return nil, []User{}, nil
			}
			users, err := cfg.UserService.ListUsers(ctx)
			if err != nil {
				return nil, nil, err
			}
			out := make([]User, len(users))
			for i, u := range users {
				out[i] = User{
					ID:                 u.ID,
					Email:              u.Email,
					Name:               u.Name,
					Timezone:           u.Timezone,
					IsAdmin:            u.IsAdmin,
					Active:             u.Active,
					MustChangePassword: u.MustChangePassword,
				}
			}
			return nil, out, nil
		})

	sdk.AddTool(s, readOnly("list_entries",
		"List entries the token holder may read: own entries for a member, own plus their teams' projects for a lead, everything for an admin. Dates are YYYY-MM-DD, from inclusive and to exclusive; both default to the current ISO week. include_deleted needs an admin."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ListEntriesInput) (*sdk.CallToolResult, []Entry, error) {
			if cfg.EntryService == nil {
				return nil, []Entry{}, nil
			}
			filter := entry.ListFilter{
				UserID:         in.UserID,
				ProjectID:      in.ProjectID,
				IncludeDeleted: in.IncludeDeleted,
			}
			if in.From != "" || in.To != "" {
				from, to, err := parseDateRange(in.From, in.To, "UTC", time.Now())
				if err != nil {
					return nil, nil, err
				}
				filter.From = &from
				filter.To = &to
			}
			entries, err := cfg.EntryService.ListEntries(ctx, filter)
			if err != nil {
				return nil, nil, err
			}
			out := make([]Entry, len(entries))
			for i, e := range entries {
				out[i] = toEntryDTO(&e)
			}
			return nil, out, nil
		})

	sdk.AddTool(s, readOnly("report",
		"Totals of logged hours per person and project. A member reads their own hours, a lead their own plus their teams' projects, an admin everything. Dates are YYYY-MM-DD, from inclusive and to exclusive; both default to the current ISO week. by is week, day or none."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ReportInput) (*sdk.CallToolResult, Report, error) {
			if cfg.ReportService == nil {
				return nil, Report{}, nil
			}
			by := in.By
			if by == "" {
				by = "week"
			}
			from, to, err := parseDateRange(in.From, in.To, "UTC", time.Now())
			if err != nil {
				return nil, Report{}, err
			}
			filter := report.Filter{
				From:      from,
				To:        to,
				By:        by,
				TeamID:    in.TeamID,
				UserID:    in.UserID,
				ProjectID: in.ProjectID,
			}
			r, err := cfg.ReportService.GenerateReport(ctx, filter)
			if err != nil {
				return nil, Report{}, err
			}
			rows := make([]ReportRow, len(r.Rows))
			for i, row := range r.Rows {
				rows[i] = ReportRow{
					Bucket:    row.Bucket,
					UserID:    row.UserID,
					User:      row.User,
					Email:     row.Email,
					TeamID:    row.TeamID,
					Team:      row.Team,
					ProjectID: row.ProjectID,
					Project:   row.Project,
					Archived:  row.Archived,
					Seconds:   row.Seconds,
				}
			}
			return nil, Report{
				Rows:         rows,
				TotalSeconds: r.TotalSeconds,
			}, nil
		})

	sdk.AddTool(s, readOnly("get_plan",
		"Planned hours per person, project and ISO week, with each person's weekly capacity and the weeks of the range. Omit from and to for next month. A member sees their own row; a lead sees their teams' projects and any other load that person carries on other teams as 'other' hours; an admin sees everything."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in PlanInput) (*sdk.CallToolResult, PlanBoard, error) {
			if cfg.PlanService == nil {
				return nil, PlanBoard{}, nil
			}
			now := time.Now()
			from, to, err := nextMonthRange(in.From, in.To, now)
			if err != nil {
				return nil, PlanBoard{}, err
			}
			board, err := cfg.PlanService.List(ctx, plan.Filter{
				From: from, To: to, TeamID: in.TeamID, UserID: in.UserID,
			})
			if err != nil {
				return nil, PlanBoard{}, err
			}
			return nil, toPlanBoard(board), nil
		})

	sdk.AddTool(s, write("set_plan",
		"Set the hours a person will spend on a project in one ISO week (0–168, 0 removes the cell). week_start is any date in that week, YYYY-MM-DD. A lead may plan people of their teams onto their teams' projects; a week that has ended in the person's timezone is refused."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in SetPlanInput) (*sdk.CallToolResult, PlanCell, error) {
			if cfg.PlanService == nil {
				return nil, PlanCell{}, nil
			}
			week, err := time.Parse("2006-01-02", in.WeekStart)
			if err != nil {
				return nil, PlanCell{}, err
			}
			cell, err := cfg.PlanService.Set(ctx, in.UserID, in.ProjectID, week, in.Hours)
			if err != nil {
				return nil, PlanCell{}, err
			}
			return nil, PlanCell{
				UserID: cell.UserID, ProjectID: cell.ProjectID,
				WeekStart: cell.WeekStart.Format("2006-01-02"), Hours: cell.Hours,
			}, nil
		})

	sdk.AddTool(s, write("seed_plan",
		"Fill a month's plan from the previous month's logged hours, split evenly across the target month's weeks and rounded to whole hours. Empty cells only; frozen weeks and existing cells are skipped. dry_run reports what would be filled without writing."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in SeedPlanInput) (*sdk.CallToolResult, PlanSummary, error) {
			if cfg.PlanService == nil {
				return nil, PlanSummary{}, nil
			}
			sum, err := cfg.PlanService.Seed(ctx, plan.SeedInput{
				FromMonth: in.FromMonth, IntoMonth: in.IntoMonth,
				TeamID: in.TeamID, DryRun: in.DryRun,
			})
			if err != nil {
				return nil, PlanSummary{}, err
			}
			return nil, toPlanSummary(sum), nil
		})

	sdk.AddTool(s, write("repeat_plan",
		"Copy one week's plan into the following weeks (1–52). Empty cells only; existing cells and frozen weeks are skipped. dry_run reports what would be filled without writing."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in RepeatPlanInput) (*sdk.CallToolResult, PlanSummary, error) {
			if cfg.PlanService == nil {
				return nil, PlanSummary{}, nil
			}
			week, err := time.Parse("2006-01-02", in.WeekStart)
			if err != nil {
				return nil, PlanSummary{}, err
			}
			sum, err := cfg.PlanService.Repeat(ctx, plan.RepeatInput{
				WeekStart: week, Weeks: in.Weeks, TeamID: in.TeamID,
				UserID: in.UserID, ProjectID: in.ProjectID, DryRun: in.DryRun,
			})
			if err != nil {
				return nil, PlanSummary{}, err
			}
			return nil, toPlanSummary(sum), nil
		})

	sdk.AddTool(s, write("start_timer",
		"Start the token holder's running entry on a project. timezone is the IANA zone it is logged in, defaulting to caller's zone. A user has at most one running entry; a second start is refused with 409 and the running entry."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in StartTimerInput) (*sdk.CallToolResult, Entry, error) {
			if cfg.EntryService == nil {
				return nil, Entry{}, nil
			}
			tz := in.Timezone
			if tz == "" && cfg.UserService != nil {
				if prof, err := cfg.UserService.GetProfile(ctx); err == nil && prof != nil && prof.User.Timezone != "" {
					tz = prof.User.Timezone
				}
			}
			e, err := cfg.EntryService.StartTimer(ctx, in.ProjectID, tz, in.Note)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, toEntryDTO(e), nil
		})

	sdk.AddTool(s, write("stop_timer",
		"Stop the token holder's running entry at the server's now, optionally adding a note. Fails if nothing runs."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in StopTimerInput) (*sdk.CallToolResult, Entry, error) {
			if cfg.EntryService == nil {
				return nil, Entry{}, nil
			}
			var notePtr *string
			if in.Note != "" {
				notePtr = &in.Note
			}
			e, err := cfg.EntryService.StopTimer(ctx, notePtr)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, toEntryDTO(e), nil
		})

	sdk.AddTool(s, write("add_entry",
		"Log a finished entry for the token holder on a project. Times are RFC 3339 instants; timezone is the IANA zone it was logged in, defaulting to caller's zone. The entry may not exceed 24 hours or start more than 5 minutes in the future. The project must be live."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in AddEntryInput) (*sdk.CallToolResult, Entry, error) {
			if cfg.EntryService == nil {
				return nil, Entry{}, nil
			}
			tz := in.Timezone
			if tz == "" && cfg.UserService != nil {
				if prof, err := cfg.UserService.GetProfile(ctx); err == nil && prof != nil && prof.User.Timezone != "" {
					tz = prof.User.Timezone
				}
			}
			e, err := cfg.EntryService.LogEntry(ctx, in.ProjectID, in.StartAt, in.EndAt, tz, in.Note)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, toEntryDTO(e), nil
		})

	sdk.AddTool(s, write("update_entry",
		"Change one of the token holder's own entries: project, start, end, timezone or note. A running entry must be given an end to be retimed. Same rules as add_entry."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in UpdateEntryInput) (*sdk.CallToolResult, Entry, error) {
			if cfg.EntryService == nil {
				return nil, Entry{}, nil
			}
			e, err := cfg.EntryService.PatchEntry(ctx, in.ID, in.ProjectID, in.StartAt, in.EndAt, in.Timezone, in.Note)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, toEntryDTO(e), nil
		})

	sdk.AddTool(s, write("delete_entry",
		"Soft-delete one of the token holder's own entries. It leaves every list and total but stays in storage; only an admin can restore it."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in DeleteEntryInput) (*sdk.CallToolResult, Deleted, error) {
			if cfg.EntryService == nil {
				return nil, Deleted{}, nil
			}
			if err := cfg.EntryService.DeleteEntry(ctx, in.ID); err != nil {
				return nil, Deleted{}, err
			}
			return nil, Deleted{Deleted: true}, nil
		})
}

// Input DTOs

// ListProjectsInput contains filters for list_projects.
type ListProjectsInput struct {
	All bool `json:"all,omitempty" jsonschema:"include archived projects (admin only)"`
}

// ListEntriesInput contains filters for list_entries.
type ListEntriesInput struct {
	From           string `json:"from,omitempty" jsonschema:"inclusive start date, YYYY-MM-DD; omit both from and to for the current ISO week"`
	To             string `json:"to,omitempty" jsonschema:"exclusive end date, YYYY-MM-DD; required with from"`
	UserID         uint   `json:"user_id,omitempty" jsonschema:"only this user's entries, within what the caller may read"`
	ProjectID      uint   `json:"project_id,omitempty" jsonschema:"only this project's entries"`
	IncludeDeleted bool   `json:"include_deleted,omitempty" jsonschema:"include soft-deleted entries (admin only)"`
}

// ReportInput contains filters for report.
type ReportInput struct {
	From      string `json:"from,omitempty" jsonschema:"inclusive start date, YYYY-MM-DD; omit both from and to for the current ISO week"`
	To        string `json:"to,omitempty" jsonschema:"exclusive end date, YYYY-MM-DD; required with from"`
	By        string `json:"by,omitempty" jsonschema:"bucket: week (default), day, month or none"`
	TeamID    uint   `json:"team_id,omitempty" jsonschema:"only entries on this team's projects"`
	UserID    uint   `json:"user_id,omitempty" jsonschema:"only this user's entries, within what the caller may read"`
	ProjectID uint   `json:"project_id,omitempty" jsonschema:"only this project's entries"`
}

// PlanInput contains filters for get_plan.
type PlanInput struct {
	From   string `json:"from,omitempty" jsonschema:"inclusive start date, YYYY-MM-DD; omit both from and to for next month"`
	To     string `json:"to,omitempty" jsonschema:"exclusive end date, YYYY-MM-DD; required with from"`
	TeamID uint   `json:"team_id,omitempty" jsonschema:"only this team's people and projects"`
	UserID uint   `json:"user_id,omitempty" jsonschema:"only this person's cells, within what the caller may read"`
}

// SetPlanInput contains arguments for set_plan.
type SetPlanInput struct {
	UserID    uint   `json:"user_id" jsonschema:"the person to plan; get_plan returns the ids"`
	ProjectID uint   `json:"project_id" jsonschema:"the project to plan against; list_projects returns the ids"`
	WeekStart string `json:"week_start" jsonschema:"any date in the ISO week, YYYY-MM-DD"`
	Hours     int    `json:"hours" jsonschema:"planned hours, 0-168; 0 removes the cell"`
}

// SeedPlanInput contains arguments for seed_plan.
type SeedPlanInput struct {
	FromMonth string `json:"from_month" jsonschema:"month whose logged hours seed the plan, YYYY-MM"`
	IntoMonth string `json:"into_month" jsonschema:"month that is planned, YYYY-MM"`
	TeamID    uint   `json:"team_id,omitempty" jsonschema:"only this team's projects"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"report what would be filled without writing"`
}

// RepeatPlanInput contains arguments for repeat_plan.
type RepeatPlanInput struct {
	WeekStart string `json:"week_start" jsonschema:"any date in the source ISO week, YYYY-MM-DD"`
	Weeks     int    `json:"weeks" jsonschema:"how many following weeks to fill, 1-52"`
	TeamID    uint   `json:"team_id,omitempty" jsonschema:"only this team's projects"`
	UserID    uint   `json:"user_id,omitempty" jsonschema:"only this person's cells"`
	ProjectID uint   `json:"project_id,omitempty" jsonschema:"only this project's cells"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"report what would be filled without writing"`
}

// StartTimerInput contains arguments for start_timer.
type StartTimerInput struct {
	ProjectID uint   `json:"project_id" jsonschema:"the project to log against; list_projects returns the ids"`
	Timezone  string `json:"timezone,omitempty" jsonschema:"IANA zone the work happens in, e.g. Australia/Sydney; omit for caller's zone"`
	Note      string `json:"note,omitempty" jsonschema:"what you are working on"`
}

// StopTimerInput contains arguments for stop_timer.
type StopTimerInput struct {
	Note string `json:"note,omitempty" jsonschema:"note to add as the entry ends"`
}

// AddEntryInput contains arguments for add_entry.
type AddEntryInput struct {
	ProjectID uint      `json:"project_id" jsonschema:"the project to log against; list_projects returns the ids"`
	StartAt   time.Time `json:"start_at" jsonschema:"RFC 3339 start instant, e.g. 2026-10-19T09:00:00+02:00"`
	EndAt     time.Time `json:"end_at" jsonschema:"RFC 3339 end instant"`
	Timezone  string    `json:"timezone,omitempty" jsonschema:"IANA zone the work happened in, e.g. Australia/Sydney; omit for caller's zone"`
	Note      string    `json:"note,omitempty" jsonschema:"what was done"`
}

// UpdateEntryInput contains arguments for update_entry.
type UpdateEntryInput struct {
	ID        uint       `json:"id" jsonschema:"the entry id from list_entries"`
	ProjectID *uint      `json:"project_id,omitempty" jsonschema:"move the entry to this project"`
	StartAt   *time.Time `json:"start_at,omitempty" jsonschema:"new RFC 3339 start instant"`
	EndAt     *time.Time `json:"end_at,omitempty" jsonschema:"new RFC 3339 end instant"`
	Timezone  *string    `json:"timezone,omitempty" jsonschema:"correct the IANA zone the entry was logged in"`
	Note      *string    `json:"note,omitempty" jsonschema:"new note; an empty string clears it"`
}

// DeleteEntryInput contains arguments for delete_entry.
type DeleteEntryInput struct {
	ID uint `json:"id" jsonschema:"the entry id from list_entries"`
}

// Deleted is returned by delete_entry.
type Deleted struct {
	Deleted bool `json:"deleted" jsonschema:"true once the entry is soft-deleted"`
}

// Output DTOs

// Membership represents user role in a team.
type Membership struct {
	TeamID uint   `json:"team_id"`
	Role   string `json:"role"`
}

// Me represents the authenticated user profile.
type Me struct {
	ID                 uint         `json:"id"`
	Email              string       `json:"email"`
	Name               string       `json:"name"`
	Timezone           string       `json:"timezone"`
	IsAdmin            bool         `json:"is_admin"`
	MustChangePassword bool         `json:"must_change_password"`
	Memberships        []Membership `json:"memberships"`
}

// Project represents a project available for logging time.
type Project struct {
	ID         uint       `json:"id"`
	TeamID     uint       `json:"team_id"`
	Name       string     `json:"name"`
	Color      string     `json:"color"`
	ArchivedAt *time.Time `json:"archived_at"`
}

// Entry represents a time tracking entry.
type Entry struct {
	ID        uint       `json:"id"`
	UserID    uint       `json:"user_id"`
	ProjectID uint       `json:"project_id"`
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at"`
	Timezone  string     `json:"timezone"`
	Note      string     `json:"note"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// TeamMember represents a user membership within a team.
type TeamMember struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// Team represents a team and its members.
type Team struct {
	ID         uint         `json:"id"`
	Name       string       `json:"name"`
	ArchivedAt *time.Time   `json:"archived_at"`
	Members    []TeamMember `json:"members"`
}

// User represents a user as shown in administration tools.
type User struct {
	ID                 uint   `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Timezone           string `json:"timezone"`
	IsAdmin            bool   `json:"is_admin"`
	Active             bool   `json:"active"`
	MustChangePassword bool   `json:"must_change_password"`
}

// ReportRow represents an aggregated row of reported time.
type ReportRow struct {
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

// Report represents aggregated time tracking data.
type Report struct {
	Rows         []ReportRow `json:"rows"`
	TotalSeconds int64       `json:"total_seconds"`
}

// PlanPerson is a person who can be planned, with their weekly capacity.
type PlanPerson struct {
	UserID      uint     `json:"user_id"`
	User        string   `json:"user"`
	Email       string   `json:"email"`
	WeeklyHours int      `json:"weekly_hours"`
	Teams       []string `json:"teams"`
	Deactivated bool     `json:"deactivated,omitempty"`
}

// PlanCell is one person, project and ISO week of planned hours.
type PlanCell struct {
	UserID    uint   `json:"user_id"`
	ProjectID uint   `json:"project_id"`
	Project   string `json:"project,omitempty"`
	Archived  bool   `json:"archived,omitempty"`
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

// nextMonthRange defaults a board to the month that has not happened yet.
func nextMonthRange(fromStr, toStr string, now time.Time) (time.Time, time.Time, error) {
	if fromStr == "" && toStr == "" {
		start := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	}
	return parseDateRange(fromStr, toStr, "UTC", now)
}

func toPlanBoard(b *plan.Board) PlanBoard {
	out := PlanBoard{Weeks: make([]string, len(b.Weeks))}
	for i, week := range b.Weeks {
		out.Weeks[i] = week.Format("2006-01-02")
	}
	out.People = make([]PlanPerson, len(b.People))
	for i, person := range b.People {
		teams := make([]string, len(person.Teams))
		for j, team := range person.Teams {
			teams[j] = team.Name
		}
		out.People[i] = PlanPerson{
			UserID: person.UserID, User: person.Name, Email: person.Email,
			WeeklyHours: person.WeeklyHours, Teams: teams, Deactivated: !person.Active,
		}
	}
	out.Cells = make([]PlanCell, len(b.Cells))
	for i, cell := range b.Cells {
		out.Cells[i] = PlanCell{
			UserID: cell.UserID, ProjectID: cell.ProjectID, Project: cell.Project,
			Archived: cell.Archived, WeekStart: cell.WeekStart.Format("2006-01-02"), Hours: cell.Hours,
		}
	}
	out.Other = make([]PlanOther, len(b.Other))
	for i, other := range b.Other {
		out.Other[i] = PlanOther{UserID: other.UserID, WeekStart: other.WeekStart.Format("2006-01-02"), Hours: other.Hours}
	}
	return out
}

func toPlanSummary(sum *plan.Summary) PlanSummary {
	out := PlanSummary{Filled: sum.Filled, Skipped: sum.Skipped}
	out.Cells = make([]PlanSummaryCell, len(sum.Cells))
	for i, cell := range sum.Cells {
		out.Cells[i] = PlanSummaryCell{
			UserID: cell.UserID, ProjectID: cell.ProjectID,
			WeekStart: cell.WeekStart.Format("2006-01-02"), Hours: cell.Hours,
		}
	}
	return out
}
