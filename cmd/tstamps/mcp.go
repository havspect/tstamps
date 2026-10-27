package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/havspect/tstamps/internal/client"
	"github.com/havspect/tstamps/internal/config"
	"github.com/havspect/tstamps/internal/mcp"
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

	if err := mcp.Serve(ctx, &clientMCPAdapter{c: client.New(cfg)}); err != nil {
		fmt.Fprintf(stderr, "tstamps mcp: %v\n", err)
		return 1
	}
	return 0
}

type clientMCPAdapter struct {
	c *client.Client
}

func (a *clientMCPAdapter) Me(_ context.Context) (*mcp.Me, error) {
	me, err := a.c.Me()
	if err != nil {
		return nil, err
	}
	memberships := make([]mcp.Membership, len(me.Memberships))
	for i, m := range me.Memberships {
		memberships[i] = mcp.Membership{TeamID: m.TeamID, Role: m.Role}
	}
	return &mcp.Me{
		ID:                 me.ID,
		Email:              me.Email,
		Name:               me.Name,
		Timezone:           me.Timezone,
		IsAdmin:            me.IsAdmin,
		MustChangePassword: me.MustChangePassword,
		Memberships:        memberships,
	}, nil
}

func (a *clientMCPAdapter) ListProjects(_ context.Context, in mcp.ListProjectsInput) ([]mcp.Project, error) {
	projects, err := a.c.ListProjects(in.All)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.Project, len(projects))
	for i, p := range projects {
		out[i] = mcp.Project{
			ID:         p.ID,
			TeamID:     p.TeamID,
			Name:       p.Name,
			Color:      p.Color,
			ArchivedAt: p.ArchivedAt,
		}
	}
	return out, nil
}

func (a *clientMCPAdapter) ListTeams(_ context.Context) ([]mcp.Team, error) {
	teams, err := a.c.ListTeams()
	if err != nil {
		return nil, err
	}
	out := make([]mcp.Team, len(teams))
	for i, t := range teams {
		members := make([]mcp.TeamMember, len(t.Members))
		for j, m := range t.Members {
			members[j] = mcp.TeamMember{
				UserID: m.UserID,
				Email:  m.Email,
				Name:   m.Name,
				Role:   m.Role,
			}
		}
		out[i] = mcp.Team{
			ID:         t.ID,
			Name:       t.Name,
			ArchivedAt: t.ArchivedAt,
			Members:    members,
		}
	}
	return out, nil
}

func (a *clientMCPAdapter) ListUsers(_ context.Context) ([]mcp.User, error) {
	users, err := a.c.ListUsers()
	if err != nil {
		return nil, err
	}
	out := make([]mcp.User, len(users))
	for i, u := range users {
		out[i] = mcp.User{
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

func (a *clientMCPAdapter) ListEntries(_ context.Context, in mcp.ListEntriesInput) ([]mcp.Entry, error) {
	entries, err := a.c.ListEntries(client.EntryQuery{
		From:           in.From,
		To:             in.To,
		UserID:         in.UserID,
		ProjectID:      in.ProjectID,
		IncludeDeleted: in.IncludeDeleted,
	})
	if err != nil {
		return nil, err
	}
	out := make([]mcp.Entry, len(entries))
	for i, e := range entries {
		out[i] = mcp.Entry{
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

func (a *clientMCPAdapter) StartTimer(_ context.Context, in mcp.StartTimerInput) (*mcp.Entry, error) {
	e, err := a.c.StartTimer(in.ProjectID, in.Timezone, in.Note)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        e.ID,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		Timezone:  e.Timezone,
		Note:      e.Note,
		DeletedAt: e.DeletedAt,
	}, nil
}

func (a *clientMCPAdapter) StopTimer(_ context.Context, in mcp.StopTimerInput) (*mcp.Entry, error) {
	e, err := a.c.StopTimer(in.Note)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        e.ID,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		Timezone:  e.Timezone,
		Note:      e.Note,
		DeletedAt: e.DeletedAt,
	}, nil
}

func (a *clientMCPAdapter) CreateEntry(_ context.Context, in mcp.AddEntryInput) (*mcp.Entry, error) {
	e, err := a.c.CreateEntry(client.EntryInput{
		ProjectID: in.ProjectID,
		StartAt:   in.StartAt,
		EndAt:     in.EndAt,
		Timezone:  in.Timezone,
		Note:      in.Note,
	})
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        e.ID,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		Timezone:  e.Timezone,
		Note:      e.Note,
		DeletedAt: e.DeletedAt,
	}, nil
}

func (a *clientMCPAdapter) PatchEntry(_ context.Context, in mcp.UpdateEntryInput) (*mcp.Entry, error) {
	e, err := a.c.PatchEntry(in.ID, client.EntryPatch{
		ProjectID: in.ProjectID,
		StartAt:   in.StartAt,
		EndAt:     in.EndAt,
		Timezone:  in.Timezone,
		Note:      in.Note,
	})
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{
		ID:        e.ID,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		Timezone:  e.Timezone,
		Note:      e.Note,
		DeletedAt: e.DeletedAt,
	}, nil
}

func (a *clientMCPAdapter) DeleteEntry(_ context.Context, in mcp.DeleteEntryInput) error {
	return a.c.DeleteEntry(in.ID)
}

func (a *clientMCPAdapter) Report(_ context.Context, in mcp.ReportInput) (*mcp.Report, error) {
	r, err := a.c.Report(client.ReportQuery{
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
	rows := make([]mcp.ReportRow, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = mcp.ReportRow{
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
	return &mcp.Report{
		Rows:         rows,
		TotalSeconds: r.TotalSeconds,
	}, nil
}
