// Package mcp exposes the Tstamps tool catalogue for MCP servers.
// It powers both the CLI stdio server and the remote Streamable HTTP server.
package mcp

import (
	"context"
	"os"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/version"
)

// Backend provides data access and domain operations for MCP tools.
type Backend interface {
	Me(ctx context.Context) (*Me, error)
	ListProjects(ctx context.Context, in ListProjectsInput) ([]Project, error)
	ListTeams(ctx context.Context) ([]Team, error)
	ListUsers(ctx context.Context) ([]User, error)
	ListEntries(ctx context.Context, in ListEntriesInput) ([]Entry, error)
	StartTimer(ctx context.Context, in StartTimerInput) (*Entry, error)
	StopTimer(ctx context.Context, in StopTimerInput) (*Entry, error)
	CreateEntry(ctx context.Context, in AddEntryInput) (*Entry, error)
	PatchEntry(ctx context.Context, in UpdateEntryInput) (*Entry, error)
	DeleteEntry(ctx context.Context, in DeleteEntryInput) error
	Report(ctx context.Context, in ReportInput) (*Report, error)
}

// Policy governs write authorization and timezone resolution for MCP tools.
type Policy interface {
	AuthorizeWrite(ctx context.Context) error
	ResolveTimezone(ctx context.Context, requested string) (string, error)
}

// Config configures the MCP server tool catalogue.
type Config struct {
	Backend Backend
	Policy  Policy
}

// Client is the interface used by the CLI stdio server.
type Client = Backend

// NewServer returns an MCP server with every tool registered using cfg.
func NewServer(cfg Config) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{
		Name: "tstamps", Title: "Tstamps", Version: version.Version,
	}, nil)
	if cfg.Policy == nil {
		cfg.Policy = &defaultPolicy{backend: cfg.Backend}
	}
	register(s, cfg)
	return s
}

// New returns the MCP server for a Client with every tool registered.
func New(c Client) *sdk.Server {
	return NewServer(Config{
		Backend: c,
		Policy:  &defaultPolicy{backend: c},
	})
}

// Serve runs the server over stdin and stdout until the client closes the pipe.
func Serve(ctx context.Context, c Client) error {
	return New(c).Run(ctx, &sdk.StdioTransport{})
}

// readOnly builds a tool whose annotation tells an agent it changes nothing.
func readOnly(name, description string) *sdk.Tool {
	return &sdk.Tool{
		Name: name, Description: description,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}
}

func write(name, description string) *sdk.Tool {
	return &sdk.Tool{Name: name, Description: description}
}

func register(s *sdk.Server, cfg Config) {
	sdk.AddTool(s, readOnly("whoami",
		"Return the authenticated user: id, email, name, timezone, admin flag and team roles. Needs only a valid token."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, Me, error) {
			me, err := cfg.Backend.Me(ctx)
			if err != nil {
				return nil, Me{}, err
			}
			return nil, *me, nil
		})

	sdk.AddTool(s, readOnly("list_projects",
		"List projects to log against, with their team and archive state. Any member may list non-archived projects; all=true includes archived projects and needs an admin."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ListProjectsInput) (*sdk.CallToolResult, []Project, error) {
			projects, err := cfg.Backend.ListProjects(ctx, in)
			return nil, projects, err
		})

	sdk.AddTool(s, readOnly("list_entries",
		"List entries the token holder may read: own entries for a member, own plus their teams' projects for a lead, everything for an admin. Dates are YYYY-MM-DD, from inclusive and to exclusive; both default to the current ISO week. include_deleted needs an admin."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ListEntriesInput) (*sdk.CallToolResult, []Entry, error) {
			entries, err := cfg.Backend.ListEntries(ctx, in)
			return nil, entries, err
		})

	sdk.AddTool(s, readOnly("report",
		"Totals of logged hours per person and project. A member reads their own hours, a lead their own plus their teams' projects, an admin everything. Dates are YYYY-MM-DD, from inclusive and to exclusive; both default to the current ISO week. by is week, day or none."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in ReportInput) (*sdk.CallToolResult, Report, error) {
			if in.By == "" {
				in.By = "week"
			}
			r, err := cfg.Backend.Report(ctx, in)
			if err != nil {
				return nil, Report{}, err
			}
			return nil, *r, nil
		})

	sdk.AddTool(s, readOnly("list_teams",
		"List every team with its members and roles. Admin only."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []Team, error) {
			teams, err := cfg.Backend.ListTeams(ctx)
			return nil, teams, err
		})

	sdk.AddTool(s, readOnly("list_users",
		"List every user with timezone, admin flag and active state. Admin only."),
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []User, error) {
			users, err := cfg.Backend.ListUsers(ctx)
			return nil, users, err
		})

	sdk.AddTool(s, write("start_timer",
		"Start the token holder's running entry on a project. timezone is the IANA zone it is logged in, defaulting to caller's zone. A user has at most one running entry; a second start is refused with 409 and the running entry."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in StartTimerInput) (*sdk.CallToolResult, Entry, error) {
			if err := cfg.Policy.AuthorizeWrite(ctx); err != nil {
				return nil, Entry{}, err
			}
			tz, err := cfg.Policy.ResolveTimezone(ctx, in.Timezone)
			if err != nil {
				return nil, Entry{}, err
			}
			in.Timezone = tz
			e, err := cfg.Backend.StartTimer(ctx, in)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, *e, nil
		})

	sdk.AddTool(s, write("stop_timer",
		"Stop the token holder's running entry at the server's now, optionally adding a note. Fails if nothing runs."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in StopTimerInput) (*sdk.CallToolResult, Entry, error) {
			if err := cfg.Policy.AuthorizeWrite(ctx); err != nil {
				return nil, Entry{}, err
			}
			e, err := cfg.Backend.StopTimer(ctx, in)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, *e, nil
		})

	sdk.AddTool(s, write("add_entry",
		"Log a finished entry for the token holder on a project. Times are RFC 3339 instants; timezone is the IANA zone it was logged in, defaulting to caller's zone. The entry may not exceed 24 hours or start more than 5 minutes in the future. The project must be live."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in AddEntryInput) (*sdk.CallToolResult, Entry, error) {
			if err := cfg.Policy.AuthorizeWrite(ctx); err != nil {
				return nil, Entry{}, err
			}
			tz, err := cfg.Policy.ResolveTimezone(ctx, in.Timezone)
			if err != nil {
				return nil, Entry{}, err
			}
			in.Timezone = tz
			e, err := cfg.Backend.CreateEntry(ctx, in)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, *e, nil
		})

	sdk.AddTool(s, write("update_entry",
		"Change one of the token holder's own entries: project, start, end, timezone or note. A running entry must be given an end to be retimed. Same rules as add_entry."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in UpdateEntryInput) (*sdk.CallToolResult, Entry, error) {
			if err := cfg.Policy.AuthorizeWrite(ctx); err != nil {
				return nil, Entry{}, err
			}
			e, err := cfg.Backend.PatchEntry(ctx, in)
			if err != nil {
				return nil, Entry{}, err
			}
			return nil, *e, nil
		})

	sdk.AddTool(s, write("delete_entry",
		"Soft-delete one of the token holder's own entries. It leaves every list and total but stays in storage; only an admin can restore it."),
		func(ctx context.Context, _ *sdk.CallToolRequest, in DeleteEntryInput) (*sdk.CallToolResult, Deleted, error) {
			if err := cfg.Policy.AuthorizeWrite(ctx); err != nil {
				return nil, Deleted{}, err
			}
			if err := cfg.Backend.DeleteEntry(ctx, in); err != nil {
				return nil, Deleted{}, err
			}
			return nil, Deleted{Deleted: true}, nil
		})
}

// Input DTOs define the tool schemas; jsonschema tags describe fields for MCP agents.

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

// Output DTOs define the structured tool output schemas.

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

type defaultPolicy struct {
	backend Backend
}

func (p *defaultPolicy) AuthorizeWrite(ctx context.Context) error {
	return nil
}

func (p *defaultPolicy) ResolveTimezone(ctx context.Context, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if p.backend != nil {
		me, err := p.backend.Me(ctx)
		if err != nil {
			return "", err
		}
		if me != nil {
			return machineTimezone(me.Timezone), nil
		}
	}
	return machineTimezone("UTC"), nil
}

func machineTimezone(fallback string) string {
	if name := os.Getenv("TZ"); name != "" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	if path, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(path, "zoneinfo/"); i >= 0 {
			name := path[i+len("zoneinfo/"):]
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return fallback
}
