// Package demo seeds the public demo workspace and resets it on the hour.
package demo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/havspect/tstamps/internal/auth"
	gormrepo "github.com/havspect/tstamps/internal/repository/gorm"
)

// Slug is the one workspace a demo server may hold.
const Slug = "demo"

// Account is a sign-in shown on the demo login page, including its password:
// the accounts are public by design.
type Account struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

var accounts = []Account{
	{Email: "ada@example.com", Name: "Ada Lovelace", Role: "admin", Password: "demo"},
	{Email: "leo@example.com", Name: "Leo Martins", Role: "lead", Password: "demo"},
	{Email: "mia@example.com", Name: "Mia Chen", Role: "member", Password: "demo"},
}

// Accounts returns the public demo sign-ins.
func Accounts() []Account {
	out := make([]Account, len(accounts))
	copy(out, accounts)
	return out
}

type person struct {
	name, email, timezone string
	weeklyHours           int
	team, role            string
	admin                 bool
}

var people = []person{
	{name: "Ada Lovelace", email: "ada@example.com", timezone: "UTC", weeklyHours: 40, team: "Product", role: "lead", admin: true},
	{name: "Leo Martins", email: "leo@example.com", timezone: "Europe/Berlin", weeklyHours: 40, team: "Product", role: "lead"},
	{name: "Mia Chen", email: "mia@example.com", timezone: "UTC", weeklyHours: 32, team: "Product", role: "member"},
	{name: "Sam Rivera", email: "sam@example.com", timezone: "America/New_York", weeklyHours: 35, team: "Product", role: "member"},
	{name: "Priya Nair", email: "priya@example.com", timezone: "Australia/Sydney", weeklyHours: 38, team: "Platform", role: "lead"},
	{name: "Tom Becker", email: "tom@example.com", timezone: "Europe/Lisbon", weeklyHours: 40, team: "Platform", role: "member"},
	{name: "Noor Haddad", email: "noor@example.com", timezone: "UTC", weeklyHours: 40, team: "Platform", role: "member"},
	{name: "Ines Duarte", email: "ines@example.com", timezone: "Europe/Madrid", weeklyHours: 40, team: "Platform", role: "member"},
}

type projectDef struct {
	team, name, color string
	archived          bool
}

var projectDefs = []projectDef{
	{team: "Product", name: "Timeline", color: "#2f5d50"},
	{team: "Product", name: "Reports", color: "#a4602c"},
	{team: "Product", name: "Integrations", color: "#4f6d9a"},
	{team: "Platform", name: "API", color: "#6b5b95"},
	{team: "Platform", name: "Mobile", color: "#b56576"},
	{team: "Platform", name: "Legacy import", color: "#8a8f8c", archived: true},
}

var blocks = []struct{ startHour, startMin, endHour, endMin int }{
	{9, 0, 10, 30},
	{10, 45, 12, 0},
	{13, 0, 14, 30},
	{14, 45, 16, 30},
}

var notes = []string{
	"Code review",
	"Pairing on the report",
	"API design",
	"Release prep",
	"Bug triage",
	"Customer call",
	"Writing docs",
	"Tests for the timeline",
}

// Seed replaces the demo workspace with fictional data relative to now. It
// refuses a database that holds any other workspace, so a mistyped --db can
// never wipe real data.
func Seed(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var ws gormrepo.Workspace
		err := tx.WithContext(gormrepo.WithBypassTenancy(ctx)).Where("slug = ?", Slug).First(&ws).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ws = gormrepo.Workspace{Slug: Slug, CreatedAt: now}
			if err := tx.WithContext(gormrepo.WithBypassTenancy(ctx)).Create(&ws).Error; err != nil {
				return fmt.Errorf("create demo workspace: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("find demo workspace: %w", err)
		}

		var others int64
		if err := tx.WithContext(gormrepo.WithBypassTenancy(ctx)).
			Model(&gormrepo.Workspace{}).Where("slug <> ?", Slug).Count(&others).Error; err != nil {
			return fmt.Errorf("count workspaces: %w", err)
		}
		if others > 0 {
			return errors.New("demo: database holds workspaces other than " + Slug)
		}

		wctx := gormrepo.WithWorkspaceID(gormrepo.WithBypassTenancy(ctx), ws.ID)
		wdb := tx.WithContext(wctx)
		if err := wipe(wdb, ws.ID); err != nil {
			return err
		}
		return fill(wdb, ws.ID, now)
	})
}

// wipe deletes the workspace's rows. audit_log is append-only through GORM and
// a SQLite trigger, so the demo reset drops that trigger, clears the rows and
// restores the trigger in the same transaction: the one deliberate exception.
func wipe(db *gorm.DB, workspaceID uint) error {
	var trigger string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'prevent_audit_log_delete'").
		Scan(&trigger).Error; err != nil {
		return fmt.Errorf("read audit trigger: %w", err)
	}
	if trigger != "" {
		if err := db.Exec("DROP TRIGGER prevent_audit_log_delete").Error; err != nil {
			return fmt.Errorf("drop audit trigger: %w", err)
		}
	}
	if err := db.Exec("DELETE FROM audit_log WHERE workspace_id = ?", workspaceID).Error; err != nil {
		return fmt.Errorf("delete audit log: %w", err)
	}
	if trigger != "" {
		if err := db.Exec(trigger).Error; err != nil {
			return fmt.Errorf("restore audit trigger: %w", err)
		}
	}
	for _, model := range []any{
		&gormrepo.OAuthRefreshToken{}, &gormrepo.OAuthAccessToken{}, &gormrepo.OAuthCode{},
		&gormrepo.OAuthDeviceCode{}, &gormrepo.OAuthClient{}, &gormrepo.Session{}, &gormrepo.APIToken{},
		&gormrepo.Plan{}, &gormrepo.Vacation{}, &gormrepo.Entry{}, &gormrepo.TeamMembership{}, &gormrepo.Project{},
		&gormrepo.Team{}, &gormrepo.User{},
	} {
		if err := db.Where("workspace_id = ?", workspaceID).Delete(model).Error; err != nil {
			return fmt.Errorf("delete %T: %w", model, err)
		}
	}
	return nil
}

func fill(db *gorm.DB, workspaceID uint, now time.Time) error {
	demoHash, err := auth.HashPassword(accounts[0].Password)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}
	// One unguessable hash for everyone who exists only to fill reports.
	hidden, err := auth.GeneratePassword()
	if err != nil {
		return fmt.Errorf("generate hidden password: %w", err)
	}
	hiddenHash, err := auth.HashPassword(hidden)
	if err != nil {
		return fmt.Errorf("hash hidden password: %w", err)
	}

	users := make([]gormrepo.User, len(people))
	for i, p := range people {
		hash := hiddenHash
		if isDemoAccount(p.email) {
			hash = demoHash
		}
		users[i] = gormrepo.User{
			WorkspaceID:  workspaceID,
			Email:        p.email,
			Name:         p.name,
			PasswordHash: hash,
			Timezone:     p.timezone,
			WeeklyHours:  p.weeklyHours,
			IsAdmin:      p.admin,
			Active:       true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	}
	if err := db.Create(&users).Error; err != nil {
		return fmt.Errorf("create users: %w", err)
	}
	userIDs := make(map[string]uint, len(users))
	for _, u := range users {
		userIDs[u.Email] = u.ID
	}

	teams := []gormrepo.Team{
		{WorkspaceID: workspaceID, Name: "Product", CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: workspaceID, Name: "Platform", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&teams).Error; err != nil {
		return fmt.Errorf("create teams: %w", err)
	}
	teamIDs := make(map[string]uint, len(teams))
	for _, t := range teams {
		teamIDs[t.Name] = t.ID
	}

	memberships := make([]gormrepo.TeamMembership, len(people))
	for i, p := range people {
		memberships[i] = gormrepo.TeamMembership{
			WorkspaceID: workspaceID,
			TeamID:      teamIDs[p.team],
			UserID:      userIDs[p.email],
			Role:        p.role,
			CreatedAt:   now,
		}
	}
	if err := db.Create(&memberships).Error; err != nil {
		return fmt.Errorf("create memberships: %w", err)
	}

	projects := make([]gormrepo.Project, len(projectDefs))
	for i, p := range projectDefs {
		projects[i] = gormrepo.Project{
			WorkspaceID: workspaceID,
			TeamID:      teamIDs[p.team],
			Name:        p.name,
			Color:       p.color,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if p.archived {
			projects[i].ArchivedAt = &now
		}
	}
	if err := db.Create(&projects).Error; err != nil {
		return fmt.Errorf("create projects: %w", err)
	}
	projectIDs := make(map[string]uint, len(projects))
	for _, p := range projects {
		projectIDs[p.Name] = p.ID
	}

	entries, err := buildEntries(now, workspaceID, userIDs, projectIDs)
	if err != nil {
		return err
	}
	if err := db.CreateInBatches(&entries, 100).Error; err != nil {
		return fmt.Errorf("create entries: %w", err)
	}

	plans := buildPlans(now, workspaceID, userIDs, projectIDs)
	if err := db.CreateInBatches(&plans, 100).Error; err != nil {
		return fmt.Errorf("create plans: %w", err)
	}
	return nil
}

// RunHourly resets the demo workspace at the top of every hour until ctx ends.
func RunHourly(ctx context.Context, db *gorm.DB) {
	for {
		now := time.Now()
		wait := time.Until(now.Truncate(time.Hour).Add(time.Hour))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := Seed(ctx, db, time.Now()); err != nil {
				slog.Error("demo reset failed", "error", err)
				continue
			}
			slog.Info("demo workspace reset")
		}
	}
}

func isDemoAccount(email string) bool {
	for _, a := range accounts {
		if a.Email == email {
			return true
		}
	}
	return false
}

func activeProjectNames(team string) []string {
	var names []string
	for _, p := range projectDefs {
		if p.team == team && !p.archived {
			names = append(names, p.name)
		}
	}
	return names
}

func buildEntries(now time.Time, workspaceID uint, userIDs, projectIDs map[string]uint) ([]gormrepo.Entry, error) {
	var out []gormrepo.Entry
	for pi, p := range people {
		loc, err := time.LoadLocation(p.timezone)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", p.timezone, err)
		}
		localNow := now.In(loc)
		weekStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).
			AddDate(0, 0, -((int(localNow.Weekday()) + 6) % 7))
		names := activeProjectNames(p.team)

		for w := 5; w >= 0; w-- {
			for d := 0; d < 5; d++ {
				day := weekStart.AddDate(0, 0, -7*w+d)
				for bi, b := range blocks {
					if (pi+d+w+bi)%5 == 0 {
						continue
					}
					start := time.Date(day.Year(), day.Month(), day.Day(), b.startHour, b.startMin, 0, 0, loc)
					end := time.Date(day.Year(), day.Month(), day.Day(), b.endHour, b.endMin, 0, 0, loc)
					if end.After(now) {
						continue
					}
					name := names[(pi+d+w+bi)%len(names)]
					out = append(out, gormrepo.Entry{
						WorkspaceID: workspaceID,
						UserID:      userIDs[p.email],
						ProjectID:   projectIDs[name],
						StartAt:     start.UTC(),
						EndAt:       ptr(end.UTC()),
						Timezone:    p.timezone,
						Note:        notes[(pi+d+w+bi)%len(notes)],
						CreatedAt:   now,
						UpdatedAt:   now,
					})
				}
			}
		}
	}

	// One entry crossing local midnight in Sydney, so reports split it.
	sydney, _ := time.LoadLocation("Australia/Sydney")
	sydneyNow := now.In(sydney)
	sydneyWeek := time.Date(sydneyNow.Year(), sydneyNow.Month(), sydneyNow.Day(), 0, 0, 0, 0, sydney).
		AddDate(0, 0, -((int(sydneyNow.Weekday()) + 6) % 7))
	night := sydneyWeek.AddDate(0, 0, -14+2)
	nightStart := time.Date(night.Year(), night.Month(), night.Day(), 22, 0, 0, 0, sydney)
	nightEnd := nightStart.Add(4 * time.Hour)
	if nightEnd.Before(now) {
		out = append(out, gormrepo.Entry{
			WorkspaceID: workspaceID,
			UserID:      userIDs["priya@example.com"],
			ProjectID:   projectIDs["Mobile"],
			StartAt:     nightStart.UTC(),
			EndAt:       ptr(nightEnd.UTC()),
			Timezone:    "Australia/Sydney",
			Note:        "Night deploy",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	// Two overlapping entries for the lead, so the timeline marks a clash.
	berlin, _ := time.LoadLocation("Europe/Berlin")
	berlinNow := now.In(berlin)
	berlinWeek := time.Date(berlinNow.Year(), berlinNow.Month(), berlinNow.Day(), 0, 0, 0, 0, berlin).
		AddDate(0, 0, -((int(berlinNow.Weekday()) + 6) % 7))
	overlapDay := berlinWeek.AddDate(0, 0, -7+1)
	for i, name := range []string{"Timeline", "Reports"} {
		start := time.Date(overlapDay.Year(), overlapDay.Month(), overlapDay.Day(), 13, i*15, 0, 0, berlin)
		out = append(out, gormrepo.Entry{
			WorkspaceID: workspaceID,
			UserID:      userIDs["leo@example.com"],
			ProjectID:   projectIDs[name],
			StartAt:     start.UTC(),
			EndAt:       ptr(start.Add(time.Hour).UTC()),
			Timezone:    "Europe/Berlin",
			Note:        "Parallel review",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	// A deleted entry for the admin to restore, and hours on the archived project.
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	lisbonNow := now.In(lisbon)
	lisbonWeek := time.Date(lisbonNow.Year(), lisbonNow.Month(), lisbonNow.Day(), 0, 0, 0, 0, lisbon).
		AddDate(0, 0, -((int(lisbonNow.Weekday()) + 6) % 7))
	deletedDay := lisbonWeek.AddDate(0, 0, -21)
	deletedStart := time.Date(deletedDay.Year(), deletedDay.Month(), deletedDay.Day(), 9, 0, 0, 0, lisbon)
	deletedEnd := deletedStart.Add(time.Hour)
	out = append(out, gormrepo.Entry{
		WorkspaceID: workspaceID,
		UserID:      userIDs["tom@example.com"],
		ProjectID:   projectIDs["API"],
		StartAt:     deletedStart.UTC(),
		EndAt:       ptr(deletedEnd.UTC()),
		Timezone:    "Europe/Lisbon",
		Note:        "Wrong project",
		CreatedAt:   now,
		UpdatedAt:   now,
		DeletedAt:   ptr(deletedEnd.UTC()),
	})

	legacyStart := time.Date(night.Year(), night.Month(), night.Day(), 9, 0, 0, 0, sydney)
	out = append(out, gormrepo.Entry{
		WorkspaceID: workspaceID,
		UserID:      userIDs["priya@example.com"],
		ProjectID:   projectIDs["Legacy import"],
		StartAt:     legacyStart.UTC(),
		EndAt:       ptr(legacyStart.Add(90 * time.Minute).UTC()),
		Timezone:    "Australia/Sydney",
		Note:        "Old records",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// The admin and the member are mid-session, so the demo opens on a running
	// entry for either account a visitor picks.
	out = append(out, gormrepo.Entry{
		WorkspaceID: workspaceID,
		UserID:      userIDs["ada@example.com"],
		ProjectID:   projectIDs["Reports"],
		StartAt:     now.Add(-12 * time.Minute).UTC(),
		Timezone:    "UTC",
		Note:        "Reviewing the week",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	out = append(out, gormrepo.Entry{
		WorkspaceID: workspaceID,
		UserID:      userIDs["mia@example.com"],
		ProjectID:   projectIDs["Timeline"],
		StartAt:     now.Add(-25 * time.Minute).UTC(),
		Timezone:    "UTC",
		Note:        "Polishing the timeline",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	return out, nil
}

func buildPlans(now time.Time, workspaceID uint, userIDs, projectIDs map[string]uint) []gormrepo.Plan {
	utc := now.UTC()
	weekStart := utc.AddDate(0, 0, -((int(utc.Weekday()) + 6) % 7))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	var out []gormrepo.Plan
	for pi, p := range people {
		names := activeProjectNames(p.team)
		for w := -4; w <= 2; w++ {
			total := 24 + ((pi+w+8)%4)*4
			first := total * 3 / 5
			for i, name := range names {
				hours := 0
				switch i {
				case 0:
					hours = first
				case 1:
					hours = total - first
				}
				if hours == 0 {
					continue
				}
				out = append(out, gormrepo.Plan{
					WorkspaceID: workspaceID,
					UserID:      userIDs[p.email],
					ProjectID:   projectIDs[name],
					WeekStart:   weekStart.AddDate(0, 0, 7*w),
					Hours:       hours,
					CreatedAt:   now,
					UpdatedAt:   now,
				})
			}
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }
