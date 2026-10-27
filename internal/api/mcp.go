package api

import (
	"context"
	"errors"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/havspect/tstamps/internal/mcp"
)

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

type apiMCPAdapter struct {
	s      *Server
	reqCtx context.Context
	ri     *requestInfo
}

func (a *apiMCPAdapter) context(ctx context.Context) context.Context {
	if requestFrom(ctx) != nil {
		return ctx
	}
	return a.reqCtx
}

func (a *apiMCPAdapter) AuthorizeWrite(_ context.Context) error {
	if !hasWriteScope(a.ri) {
		return errors.New("insufficient scope: tstamps.write required")
	}
	return nil
}

func (a *apiMCPAdapter) ResolveTimezone(_ context.Context, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if a.ri != nil && a.ri.User != nil && a.ri.User.Timezone != "" {
		return a.ri.User.Timezone, nil
	}
	return "UTC", nil
}

func (a *apiMCPAdapter) Me(ctx context.Context) (*mcp.Me, error) {
	out, err := a.s.me(a.context(ctx), nil)
	if err != nil {
		return nil, err
	}
	b := out.Body
	memberships := make([]mcp.Membership, len(b.Memberships))
	for i, m := range b.Memberships {
		memberships[i] = mcp.Membership{TeamID: m.TeamID, Role: m.Role}
	}
	return &mcp.Me{
		ID:                 b.ID,
		Email:              b.Email,
		Name:               b.Name,
		Timezone:           b.Timezone,
		IsAdmin:            b.IsAdmin,
		MustChangePassword: b.MustChangePassword,
		Memberships:        memberships,
	}, nil
}

func (a *apiMCPAdapter) ListProjects(ctx context.Context, in mcp.ListProjectsInput) ([]mcp.Project, error) {
	out, err := a.s.listProjects(a.context(ctx), &projectListInput{All: in.All})
	if err != nil {
		return nil, err
	}
	projects := make([]mcp.Project, len(out.Body))
	for i, p := range out.Body {
		projects[i] = mcp.Project{
			ID:         p.ID,
			TeamID:     p.TeamID,
			Name:       p.Name,
			Color:      p.Color,
			ArchivedAt: p.ArchivedAt,
		}
	}
	return projects, nil
}

func (a *apiMCPAdapter) ListTeams(ctx context.Context) ([]mcp.Team, error) {
	out, err := a.s.listTeams(a.context(ctx), nil)
	if err != nil {
		return nil, err
	}
	teams := make([]mcp.Team, len(out.Body))
	for i, t := range out.Body {
		members := make([]mcp.TeamMember, len(t.Members))
		for j, m := range t.Members {
			members[j] = mcp.TeamMember{
				UserID: m.UserID,
				Email:  m.Email,
				Name:   m.Name,
				Role:   m.Role,
			}
		}
		teams[i] = mcp.Team{
			ID:         t.ID,
			Name:       t.Name,
			ArchivedAt: t.ArchivedAt,
			Members:    members,
		}
	}
	return teams, nil
}

func (a *apiMCPAdapter) ListUsers(ctx context.Context) ([]mcp.User, error) {
	out, err := a.s.listUsers(a.context(ctx), nil)
	if err != nil {
		return nil, err
	}
	users := make([]mcp.User, len(out.Body))
	for i, u := range out.Body {
		users[i] = mcp.User{
			ID:                 u.ID,
			Email:              u.Email,
			Name:               u.Name,
			Timezone:           u.Timezone,
			IsAdmin:            u.IsAdmin,
			Active:             u.Active,
			MustChangePassword: u.MustChangePassword,
		}
	}
	return users, nil
}

func (a *apiMCPAdapter) ListEntries(ctx context.Context, in mcp.ListEntriesInput) ([]mcp.Entry, error) {
	out, err := a.s.listEntries(a.context(ctx), &entryListInput{
		From:           in.From,
		To:             in.To,
		UserID:         in.UserID,
		ProjectID:      in.ProjectID,
		IncludeDeleted: in.IncludeDeleted,
	})
	if err != nil {
		return nil, err
	}
	entries := make([]mcp.Entry, len(out.Body))
	for i, e := range out.Body {
		entries[i] = mcp.Entry{
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
	return entries, nil
}

func (a *apiMCPAdapter) StartTimer(ctx context.Context, in mcp.StartTimerInput) (*mcp.Entry, error) {
	inReq := &timerStartInput{}
	inReq.Body.ProjectID = in.ProjectID
	inReq.Body.Timezone = in.Timezone
	inReq.Body.Note = in.Note
	out, err := a.s.startTimer(a.context(ctx), inReq)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        out.Body.ID,
		UserID:    out.Body.UserID,
		ProjectID: out.Body.ProjectID,
		StartAt:   out.Body.StartAt,
		EndAt:     out.Body.EndAt,
		Timezone:  out.Body.Timezone,
		Note:      out.Body.Note,
		DeletedAt: out.Body.DeletedAt,
	}, nil
}

func (a *apiMCPAdapter) StopTimer(ctx context.Context, in mcp.StopTimerInput) (*mcp.Entry, error) {
	inReq := &timerStopInput{Body: &struct {
		Note string `json:"note,omitempty"`
	}{Note: in.Note}}
	out, err := a.s.stopTimer(a.context(ctx), inReq)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        out.Body.ID,
		UserID:    out.Body.UserID,
		ProjectID: out.Body.ProjectID,
		StartAt:   out.Body.StartAt,
		EndAt:     out.Body.EndAt,
		Timezone:  out.Body.Timezone,
		Note:      out.Body.Note,
		DeletedAt: out.Body.DeletedAt,
	}, nil
}

func (a *apiMCPAdapter) CreateEntry(ctx context.Context, in mcp.AddEntryInput) (*mcp.Entry, error) {
	inReq := &entryCreateInput{}
	inReq.Body.ProjectID = in.ProjectID
	inReq.Body.StartAt = in.StartAt
	inReq.Body.EndAt = in.EndAt
	inReq.Body.Timezone = in.Timezone
	inReq.Body.Note = in.Note
	out, err := a.s.createEntry(a.context(ctx), inReq)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        out.Body.ID,
		UserID:    out.Body.UserID,
		ProjectID: out.Body.ProjectID,
		StartAt:   out.Body.StartAt,
		EndAt:     out.Body.EndAt,
		Timezone:  out.Body.Timezone,
		Note:      out.Body.Note,
		DeletedAt: out.Body.DeletedAt,
	}, nil
}

func (a *apiMCPAdapter) PatchEntry(ctx context.Context, in mcp.UpdateEntryInput) (*mcp.Entry, error) {
	inReq := &entryPatchInput{ID: in.ID}
	inReq.Body.ProjectID = in.ProjectID
	inReq.Body.StartAt = in.StartAt
	inReq.Body.EndAt = in.EndAt
	inReq.Body.Timezone = in.Timezone
	inReq.Body.Note = in.Note
	out, err := a.s.patchEntry(a.context(ctx), inReq)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        out.Body.ID,
		UserID:    out.Body.UserID,
		ProjectID: out.Body.ProjectID,
		StartAt:   out.Body.StartAt,
		EndAt:     out.Body.EndAt,
		Timezone:  out.Body.Timezone,
		Note:      out.Body.Note,
		DeletedAt: out.Body.DeletedAt,
	}, nil
}

func (a *apiMCPAdapter) DeleteEntry(ctx context.Context, in mcp.DeleteEntryInput) error {
	_, err := a.s.deleteEntry(a.context(ctx), &entryDeleteInput{ID: in.ID})
	return err
}

func (a *apiMCPAdapter) Report(ctx context.Context, in mcp.ReportInput) (*mcp.Report, error) {
	out, err := a.s.getReport(a.context(ctx), &reportInput{
		From:      in.From,
		To:        in.To,
		By:        in.By,
		TeamID:    in.TeamID,
		UserID:    in.UserID,
		ProjectID: in.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	rows := make([]mcp.ReportRow, len(out.Body.Rows))
	for i, r := range out.Body.Rows {
		rows[i] = mcp.ReportRow{
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
	return &mcp.Report{
		Rows:         rows,
		TotalSeconds: out.Body.TotalSeconds,
	}, nil
}

func (s *Server) newRemoteMCPServer(reqCtx context.Context) *sdk.Server {
	ri := requestFrom(reqCtx)
	adapter := &apiMCPAdapter{s: s, reqCtx: reqCtx, ri: ri}
	return mcp.NewServer(mcp.Config{
		Backend: adapter,
		Policy:  adapter,
	})
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
