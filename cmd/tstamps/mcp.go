package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/havspect/tstamps/internal/client"
	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/entry"
	"github.com/havspect/tstamps/internal/mcpserver"
	"github.com/havspect/tstamps/internal/project"
	"github.com/havspect/tstamps/internal/report"
	"github.com/havspect/tstamps/internal/team"
	"github.com/havspect/tstamps/internal/user"
)

// mcpCommand runs the MCP server over stdio, with the same config and token as
// every other client command. Nothing may be written to stdout but protocol
// frames, so diagnostics go to stderr.
func mcpCommand(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the caller reports the error in one line
	urlFlag := fs.String("url", "", "server URL")
	tokenFlag := fs.String("token", "", "personal API token")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "tstamps mcp: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: tstamps mcp [--url URL] [--token TOKEN]")
		return 2
	}

	cfg, err := config.LoadClient(*urlFlag, *tokenFlag, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tstamps mcp: %v\n", err)
		return 1
	}
	if cfg.URL == "" {
		fmt.Fprintln(stderr, "tstamps mcp: no server URL; run tstamps login --url URL or set TSTAMPS_URL")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := client.New(cfg)
	mcpCfg := newClientMCPConfig(c)
	if err := mcpserver.ServeStdio(ctx, mcpCfg); err != nil {
		fmt.Fprintf(stderr, "tstamps mcp: %v\n", err)
		return 1
	}
	return 0
}

func newClientMCPConfig(c *client.Client) mcpserver.Config {
	return mcpserver.Config{
		UserService:    &clientUserService{c: c},
		ProjectService: &clientProjectService{c: c},
		TeamService:    &clientTeamService{c: c},
		EntryService:   &clientEntryService{c: c},
		ReportService:  &clientReportService{c: c},
	}
}

type clientUserService struct{ c *client.Client }

func (s *clientUserService) GetProfile(_ context.Context) (*user.Profile, error) {
	me, err := s.c.Me()
	if err != nil {
		return nil, err
	}
	led := make([]uint, 0)
	for _, m := range me.Memberships {
		if m.Role == "lead" {
			led = append(led, m.TeamID)
		}
	}
	return &user.Profile{
		User: user.User{
			ID:                 me.ID,
			Email:              me.Email,
			Name:               me.Name,
			Timezone:           me.Timezone,
			IsAdmin:            me.IsAdmin,
			MustChangePassword: me.MustChangePassword,
		},
		LedTeamIDs: led,
	}, nil
}

func (s *clientUserService) ListUsers(_ context.Context) ([]user.User, error) {
	users, err := s.c.ListUsers()
	if err != nil {
		return nil, err
	}
	out := make([]user.User, len(users))
	for i, u := range users {
		out[i] = user.User{
			ID:                 u.ID,
			Email:              u.Email,
			Name:               u.Name,
			Timezone:           u.Timezone,
			IsAdmin:            u.IsAdmin,
			Active:             u.Active,
			MustChangePassword: u.MustChangePassword,
		}
	}
	return out, nil
}

type clientProjectService struct{ c *client.Client }

func (s *clientProjectService) ListProjects(_ context.Context, includeArchived bool) ([]project.Project, error) {
	projects, err := s.c.ListProjects(includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]project.Project, len(projects))
	for i, p := range projects {
		out[i] = project.Project{
			ID:         p.ID,
			TeamID:     p.TeamID,
			Name:       p.Name,
			Color:      p.Color,
			ArchivedAt: p.ArchivedAt,
		}
	}
	return out, nil
}

type clientTeamService struct{ c *client.Client }

func (s *clientTeamService) ListTeams(_ context.Context, _ bool) ([]team.TeamWithMembers, error) {
	teams, err := s.c.ListTeams()
	if err != nil {
		return nil, err
	}
	out := make([]team.TeamWithMembers, len(teams))
	for i, t := range teams {
		members := make([]team.Membership, len(t.Members))
		for j, m := range t.Members {
			members[j] = team.Membership{
				TeamID: t.ID,
				UserID: m.UserID,
				Role:   m.Role,
			}
		}
		out[i] = team.TeamWithMembers{
			Team: team.Team{
				ID:         t.ID,
				Name:       t.Name,
				ArchivedAt: t.ArchivedAt,
			},
			Members: members,
		}
	}
	return out, nil
}

type clientEntryService struct{ c *client.Client }

func (s *clientEntryService) ListEntries(_ context.Context, filter entry.ListFilter) ([]entry.Entry, error) {
	q := client.EntryQuery{
		UserID:         filter.UserID,
		ProjectID:      filter.ProjectID,
		IncludeDeleted: filter.IncludeDeleted,
	}
	if filter.From != nil {
		q.From = filter.From.Format("2006-01-02")
	}
	if filter.To != nil {
		q.To = filter.To.Format("2006-01-02")
	}
	entries, err := s.c.ListEntries(q)
	if err != nil {
		return nil, err
	}
	out := make([]entry.Entry, len(entries))
	for i, e := range entries {
		out[i] = entry.Entry{
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
	return out, nil
}

func (s *clientEntryService) StartTimer(_ context.Context, projectID uint, tz, note string) (*entry.Entry, error) {
	e, err := s.c.StartTimer(projectID, tz, note)
	if err != nil {
		return nil, err
	}
	return toDomainEntry(e), nil
}

func (s *clientEntryService) StopTimer(_ context.Context, note *string) (*entry.Entry, error) {
	n := ""
	if note != nil {
		n = *note
	}
	e, err := s.c.StopTimer(n)
	if err != nil {
		return nil, err
	}
	return toDomainEntry(e), nil
}

func (s *clientEntryService) LogEntry(_ context.Context, projectID uint, start, end time.Time, tz, note string) (*entry.Entry, error) {
	e, err := s.c.CreateEntry(client.EntryInput{
		ProjectID: projectID,
		StartAt:   start,
		EndAt:     end,
		Timezone:  tz,
		Note:      note,
	})
	if err != nil {
		return nil, err
	}
	return toDomainEntry(e), nil
}

func (s *clientEntryService) PatchEntry(_ context.Context, id uint, projectID *uint, start, end *time.Time, timezone *string, note *string) (*entry.Entry, error) {
	p := client.EntryPatch{
		ProjectID: projectID,
		StartAt:   start,
		EndAt:     end,
		Timezone:  timezone,
		Note:      note,
	}
	e, err := s.c.PatchEntry(id, p)
	if err != nil {
		return nil, err
	}
	return toDomainEntry(e), nil
}

func (s *clientEntryService) DeleteEntry(_ context.Context, id uint) error {
	return s.c.DeleteEntry(id)
}

func toDomainEntry(e *client.Entry) *entry.Entry {
	if e == nil {
		return nil
	}
	return &entry.Entry{
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

type clientReportService struct{ c *client.Client }

func (s *clientReportService) GenerateReport(_ context.Context, filter report.Filter) (*report.Report, error) {
	r, err := s.c.Report(client.ReportQuery{
		From:      filter.From.Format("2006-01-02"),
		To:        filter.To.Format("2006-01-02"),
		By:        filter.By,
		TeamID:    filter.TeamID,
		UserID:    filter.UserID,
		ProjectID: filter.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	rows := make([]report.Row, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = report.Row{
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
	return &report.Report{
		Rows:         rows,
		TotalSeconds: r.TotalSeconds,
	}, nil
}
