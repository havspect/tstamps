package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/version"
)

type mcpListProjectsInput struct {
	All bool `json:"all,omitempty" jsonschema:"include archived projects (admin only)"`
}

type mcpListEntriesInput struct {
	From           string `json:"from,omitempty" jsonschema:"inclusive start date, YYYY-MM-DD; omit both from and to for the current ISO week"`
	To             string `json:"to,omitempty" jsonschema:"exclusive end date, YYYY-MM-DD; required with from"`
	UserID         uint   `json:"user_id,omitempty" jsonschema:"only this user's entries, within what the caller may read"`
	ProjectID      uint   `json:"project_id,omitempty" jsonschema:"only this project's entries"`
	IncludeDeleted bool   `json:"include_deleted,omitempty" jsonschema:"include soft-deleted entries (admin only)"`
}

type mcpReportInput struct {
	From      string `json:"from,omitempty" jsonschema:"inclusive start date, YYYY-MM-DD; omit both from and to for the current ISO week"`
	To        string `json:"to,omitempty" jsonschema:"exclusive end date, YYYY-MM-DD; required with from"`
	By        string `json:"by,omitempty" jsonschema:"bucket: week (default), day, month or none"`
	TeamID    uint   `json:"team_id,omitempty" jsonschema:"only entries on this team's projects"`
	UserID    uint   `json:"user_id,omitempty" jsonschema:"only this user's entries, within what the caller may read"`
	ProjectID uint   `json:"project_id,omitempty" jsonschema:"only this project's entries"`
}

type mcpStartTimerInput struct {
	ProjectID uint   `json:"project_id" jsonschema:"the project to log against; list_projects returns the ids"`
	Timezone  string `json:"timezone,omitempty" jsonschema:"IANA zone the work happens in, e.g. Australia/Sydney; omit for user profile zone"`
	Note      string `json:"note,omitempty" jsonschema:"what you are working on"`
}

type mcpStopTimerInput struct {
	Note string `json:"note,omitempty" jsonschema:"note to add as the entry ends"`
}

type mcpAddEntryInput struct {
	ProjectID uint      `json:"project_id" jsonschema:"the project to log against; list_projects returns the ids"`
	StartAt   time.Time `json:"start_at" jsonschema:"RFC 3339 start instant, e.g. 2026-10-19T09:00:00+02:00"`
	EndAt     time.Time `json:"end_at" jsonschema:"RFC 3339 end instant"`
	Timezone  string    `json:"timezone,omitempty" jsonschema:"IANA zone the work happened in, e.g. Australia/Sydney; omit for user profile zone"`
	Note      string    `json:"note,omitempty" jsonschema:"what was done"`
}

type mcpUpdateEntryInput struct {
	ID        uint       `json:"id" jsonschema:"the entry id from list_entries"`
	ProjectID *uint      `json:"project_id,omitempty" jsonschema:"move the entry to this project"`
	StartAt   *time.Time `json:"start_at,omitempty" jsonschema:"new RFC 3339 start instant"`
	EndAt     *time.Time `json:"end_at,omitempty" jsonschema:"new RFC 3339 end instant"`
	Timezone  *string    `json:"timezone,omitempty" jsonschema:"correct the IANA zone the entry was logged in"`
	Note      *string    `json:"note,omitempty" jsonschema:"new note; an empty string clears it"`
}

type mcpDeleteEntryInput struct {
	ID uint `json:"id" jsonschema:"the entry id from list_entries"`
}

type mcpDeleted struct {
	Deleted bool `json:"deleted"`
}

type mcpReportResult struct {
	Rows         []reportRow `json:"rows"`
	TotalSeconds int64       `json:"total_seconds"`
}

func mcpReadOnly(name, description string) *sdk.Tool {
	return &sdk.Tool{
		Name: name, Description: description,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}
}

func mcpWrite(name, description string) *sdk.Tool {
	return &sdk.Tool{Name: name, Description: description}
}

func hasWriteScope(ri *requestInfo) bool {
	if ri == nil {
		return false
	}
	if ri.Session != nil || ri.Token != nil {
		return true
	}
	for _, sc := range ri.Scopes {
		if sc == "tstamps.write" {
			return true
		}
	}
	return false
}

func (s *Server) newRemoteMCPServer(reqCtx context.Context) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{
		Name: "tstamps", Title: "Tstamps", Version: version.Version,
	}, nil)
	ri := requestFrom(reqCtx)

	sdk.AddTool(srv, mcpReadOnly("whoami",
		"Return the authenticated user: id, email, name, timezone, admin flag and team roles."),
		func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, meResponse, error) {
			out, err := s.me(reqCtx, nil)
			if err != nil {
				return nil, meResponse{}, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpReadOnly("list_projects",
		"List projects to log against, with their team and archive state."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpListProjectsInput) (*sdk.CallToolResult, []projectResponse, error) {
			out, err := s.listProjects(reqCtx, &projectListInput{All: in.All})
			if err != nil {
				return nil, nil, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpReadOnly("list_entries",
		"List entries the token holder may read."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpListEntriesInput) (*sdk.CallToolResult, []entryResponse, error) {
			out, err := s.listEntries(reqCtx, &entryListInput{
				From: in.From, To: in.To, UserID: in.UserID,
				ProjectID: in.ProjectID, IncludeDeleted: in.IncludeDeleted,
			})
			if err != nil {
				return nil, nil, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpReadOnly("report",
		"Totals of logged hours per person and project."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpReportInput) (*sdk.CallToolResult, mcpReportResult, error) {
			by := in.By
			if by == "" {
				by = "week"
			}
			out, err := s.getReport(reqCtx, &reportInput{
				From: in.From, To: in.To, By: by,
				TeamID: in.TeamID, UserID: in.UserID, ProjectID: in.ProjectID,
			})
			if err != nil {
				return nil, mcpReportResult{}, err
			}
			return nil, mcpReportResult{Rows: out.Body.Rows, TotalSeconds: out.Body.TotalSeconds}, nil
		})

	sdk.AddTool(srv, mcpReadOnly("list_teams",
		"List every team with its members and roles. Admin only."),
		func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []teamResponse, error) {
			out, err := s.listTeams(reqCtx, nil)
			if err != nil {
				return nil, nil, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpReadOnly("list_users",
		"List every user with timezone, admin flag and active state. Admin only."),
		func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []userResponse, error) {
			out, err := s.listUsers(reqCtx, nil)
			if err != nil {
				return nil, nil, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpWrite("start_timer",
		"Start the token holder's running entry on a project."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpStartTimerInput) (*sdk.CallToolResult, entryResponse, error) {
			if !hasWriteScope(ri) {
				return nil, entryResponse{}, errors.New("insufficient scope: tstamps.write required")
			}
			tz := in.Timezone
			if tz == "" && ri != nil && ri.User != nil {
				tz = ri.User.Timezone
			}
			if tz == "" {
				tz = "UTC"
			}
			inReq := &timerStartInput{}
			inReq.Body.ProjectID = in.ProjectID
			inReq.Body.Timezone = tz
			inReq.Body.Note = in.Note
			out, err := s.startTimer(reqCtx, inReq)
			if err != nil {
				return nil, entryResponse{}, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpWrite("stop_timer",
		"Stop the token holder's running entry at the server's now."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpStopTimerInput) (*sdk.CallToolResult, entryResponse, error) {
			if !hasWriteScope(ri) {
				return nil, entryResponse{}, errors.New("insufficient scope: tstamps.write required")
			}
			inReq := &timerStopInput{Body: &struct {
				Note string `json:"note,omitempty"`
			}{Note: in.Note}}
			out, err := s.stopTimer(reqCtx, inReq)
			if err != nil {
				return nil, entryResponse{}, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpWrite("add_entry",
		"Log a finished entry for the token holder on a project."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpAddEntryInput) (*sdk.CallToolResult, entryResponse, error) {
			if !hasWriteScope(ri) {
				return nil, entryResponse{}, errors.New("insufficient scope: tstamps.write required")
			}
			tz := in.Timezone
			if tz == "" && ri != nil && ri.User != nil {
				tz = ri.User.Timezone
			}
			if tz == "" {
				tz = "UTC"
			}
			inReq := &entryCreateInput{}
			inReq.Body.ProjectID = in.ProjectID
			inReq.Body.StartAt = in.StartAt
			inReq.Body.EndAt = in.EndAt
			inReq.Body.Timezone = tz
			inReq.Body.Note = in.Note
			out, err := s.createEntry(reqCtx, inReq)
			if err != nil {
				return nil, entryResponse{}, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpWrite("update_entry",
		"Change one of the token holder's own entries."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpUpdateEntryInput) (*sdk.CallToolResult, entryResponse, error) {
			if !hasWriteScope(ri) {
				return nil, entryResponse{}, errors.New("insufficient scope: tstamps.write required")
			}
			inReq := &entryPatchInput{ID: in.ID}
			inReq.Body.ProjectID = in.ProjectID
			inReq.Body.StartAt = in.StartAt
			inReq.Body.EndAt = in.EndAt
			inReq.Body.Timezone = in.Timezone
			inReq.Body.Note = in.Note
			out, err := s.patchEntry(reqCtx, inReq)
			if err != nil {
				return nil, entryResponse{}, err
			}
			return nil, out.Body, nil
		})

	sdk.AddTool(srv, mcpWrite("delete_entry",
		"Soft-delete one of the token holder's own entries."),
		func(_ context.Context, _ *sdk.CallToolRequest, in mcpDeleteEntryInput) (*sdk.CallToolResult, mcpDeleted, error) {
			if !hasWriteScope(ri) {
				return nil, mcpDeleted{}, errors.New("insufficient scope: tstamps.write required")
			}
			_, err := s.deleteEntry(reqCtx, &entryDeleteInput{ID: in.ID})
			if err != nil {
				return nil, mcpDeleted{}, err
			}
			return nil, mcpDeleted{Deleted: true}, nil
		})

	return srv
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	ri := requestFrom(r.Context())
	if ri == nil || ri.User == nil {
		issuer := issuerURL(r)
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+issuer+`/.well-known/oauth-protected-resource"`)
		writeProblem(w, problem(http.StatusUnauthorized, urnUnauthorized, "authentication required"))
		return
	}

	handler := sdk.NewStreamableHTTPHandler(func(req *http.Request) *sdk.Server {
		return s.newRemoteMCPServer(req.Context())
	}, &sdk.StreamableHTTPOptions{Stateless: true})
	handler.ServeHTTP(w, r)
}
