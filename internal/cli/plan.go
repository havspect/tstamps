package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/havspect/tstamps/internal/client"
	"github.com/havspect/tstamps/internal/plan"
)

func (a *App) plan(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "set":
			return a.planSet(args[1:])
		case "rm":
			return a.planRm(args[1:])
		case "seed":
			return a.planSeed(args[1:])
		case "repeat":
			return a.planRepeat(args[1:])
		}
	}
	return a.planList(args)
}

// planRange is the default board: next month in the machine's zone.
func (a *App) planRange() (string, string) {
	loc := a.Local
	if loc == nil {
		loc = time.Local
	}
	local := a.Now().In(loc)
	start := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02")
}

func (a *App) planList(args []string) int {
	fs := newFlags("plan")
	urlFlag, tokenFlag := clientFlags(fs)
	from := fs.String("from", "", "inclusive start date, YYYY-MM-DD (default next month)")
	to := fs.String("to", "", "exclusive end date, YYYY-MM-DD")
	team := fs.String("team", "", "team name or id")
	user := fs.String("user", "", "me, an id, or an email (email needs an admin)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("plan", err)
	}
	if code, ok := a.checkNoArgs("plan", fs); !ok {
		return code
	}

	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("plan", err)
	}
	q := client.PlanQuery{From: *from, To: *to}
	if q.From == "" && q.To == "" {
		q.From, q.To = a.planRange()
	}
	if *user != "" {
		if q.UserID, err = a.resolveUser(c, *user); err != nil {
			return a.fail("plan", err)
		}
	}
	if *team != "" {
		me, err := c.Me()
		if err != nil {
			return a.fail("plan", err)
		}
		if q.TeamID, err = a.resolveTeam(c, me, *team); err != nil {
			return a.fail("plan", err)
		}
	}

	board, err := c.Plan(q)
	if err != nil {
		return a.fail("plan", err)
	}
	if *asJSON {
		return a.writeJSON(board)
	}
	a.printPlan(board)
	return 0
}

func (a *App) printPlan(board *client.PlanBoard) {
	projects := make(map[uint]string, len(board.Cells))
	for _, cell := range board.Cells {
		projects[cell.ProjectID] = cell.Project
	}
	people := make(map[uint]string, len(board.People))
	capacity := 0
	for _, person := range board.People {
		people[person.UserID] = person.User
		if len(person.Weeks) > 0 {
			for _, week := range person.Weeks {
				capacity += week.Capacity
			}
		} else {
			capacity += person.WeeklyHours * len(board.Weeks)
		}
	}
	planned := 0
	for _, cell := range board.Cells {
		planned += cell.Hours
	}
	for _, other := range board.Other {
		planned += other.Hours
	}

	w := newTable(a.Out)
	fmt.Fprintln(w, "WEEK\tUSER\tPROJECT\tHOURS")
	for _, cell := range board.Cells {
		project := projectName(projects, cell.ProjectID)
		if cell.Archived {
			project += " (archived)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", isoWeek(cell.WeekStart), personName(people, cell.UserID), project, cell.Hours)
	}
	for _, other := range board.Other {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", isoWeek(other.WeekStart), personName(people, other.UserID), "(other teams)", other.Hours)
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%d of %d\n", "", "total", "planned", planned, capacity)
	_ = w.Flush()
}

func (a *App) planSet(args []string) int {
	fs := newFlags("plan set")
	urlFlag, tokenFlag := clientFlags(fs)
	week := fs.String("week", "", "ISO week (2026-W46) or any date in it")
	hours := fs.Int("hours", -1, "planned hours, 0..168")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("plan set", err)
	}
	if fs.NArg() != 2 || *week == "" || *hours < 0 {
		return a.usageLine("tstamps plan set <person> <project> --week 2026-W46 --hours 20")
	}
	start, err := parseWeekStart(*week)
	if err != nil {
		return a.fail("plan set", err)
	}

	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("plan set", err)
	}
	userID, err := a.resolveUser(c, fs.Arg(0))
	if err != nil {
		return a.fail("plan set", err)
	}
	project, err := a.resolveProject(c, fs.Arg(1))
	if err != nil {
		return a.fail("plan set", err)
	}
	cell, err := c.SetPlan(client.PlanCellInput{
		UserID: userID, ProjectID: project.ID, WeekStart: start.Format("2006-01-02"), Hours: *hours,
	})
	if err != nil {
		return a.fail("plan set", err)
	}
	fmt.Fprintf(a.Out, "planned %d h for user %d on %s in %s\n", cell.Hours, cell.UserID, project.Name, isoWeek(cell.WeekStart))
	return 0
}

func (a *App) planRm(args []string) int {
	fs := newFlags("plan rm")
	urlFlag, tokenFlag := clientFlags(fs)
	week := fs.String("week", "", "ISO week (2026-W46) or any date in it")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("plan rm", err)
	}
	if fs.NArg() != 2 || *week == "" {
		return a.usageLine("tstamps plan rm <person> <project> --week 2026-W46")
	}
	start, err := parseWeekStart(*week)
	if err != nil {
		return a.fail("plan rm", err)
	}

	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("plan rm", err)
	}
	userID, err := a.resolveUser(c, fs.Arg(0))
	if err != nil {
		return a.fail("plan rm", err)
	}
	project, err := a.resolveProject(c, fs.Arg(1))
	if err != nil {
		return a.fail("plan rm", err)
	}
	if _, err := c.SetPlan(client.PlanCellInput{
		UserID: userID, ProjectID: project.ID, WeekStart: start.Format("2006-01-02"), Hours: 0,
	}); err != nil {
		return a.fail("plan rm", err)
	}
	fmt.Fprintf(a.Out, "cleared user %d on %s in %s\n", userID, project.Name, isoWeek(start.Format("2006-01-02")))
	return 0
}

func (a *App) planSeed(args []string) int {
	fs := newFlags("plan seed")
	urlFlag, tokenFlag := clientFlags(fs)
	from := fs.String("from", "", "month whose logged hours seed the plan, YYYY-MM")
	into := fs.String("into", "", "month that is planned, YYYY-MM")
	team := fs.String("team", "", "team name or id")
	dryRun := fs.Bool("dry-run", false, "report what would be filled without writing")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("plan seed", err)
	}
	if *from == "" || *into == "" {
		return a.usageLine("tstamps plan seed --from 2026-10 --into 2026-11 [--team ...] [--dry-run] [--json]")
	}

	in := client.SeedPlanInput{FromMonth: *from, IntoMonth: *into, DryRun: *dryRun}
	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("plan seed", err)
	}
	if *team != "" {
		me, err := c.Me()
		if err != nil {
			return a.fail("plan seed", err)
		}
		if in.TeamID, err = a.resolveTeam(c, me, *team); err != nil {
			return a.fail("plan seed", err)
		}
	}
	sum, err := c.SeedPlan(in)
	if err != nil {
		return a.fail("plan seed", err)
	}
	return a.printPlanSummary("plan seed", sum, *asJSON, *dryRun)
}

func (a *App) planRepeat(args []string) int {
	fs := newFlags("plan repeat")
	urlFlag, tokenFlag := clientFlags(fs)
	week := fs.String("week", "", "source ISO week (2026-W46) or any date in it")
	weeks := fs.Int("weeks", 4, "how many following weeks to fill")
	team := fs.String("team", "", "team name or id")
	user := fs.String("user", "", "me, an id, or an email (email needs an admin)")
	project := fs.String("project", "", "project name or id")
	dryRun := fs.Bool("dry-run", false, "report what would be filled without writing")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("plan repeat", err)
	}
	if *week == "" || *weeks < 1 || *weeks > plan.MaxRepeatWeeks {
		return a.usageLine("tstamps plan repeat --week 2026-W46 --weeks 4 [--team ...] [--user ...] [--project ...] [--dry-run] [--json]")
	}
	start, err := parseWeekStart(*week)
	if err != nil {
		return a.fail("plan repeat", err)
	}

	in := client.RepeatPlanInput{WeekStart: start.Format("2006-01-02"), Weeks: *weeks, DryRun: *dryRun}
	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("plan repeat", err)
	}
	if *user != "" {
		if in.UserID, err = a.resolveUser(c, *user); err != nil {
			return a.fail("plan repeat", err)
		}
	}
	if *project != "" {
		p, err := a.resolveProject(c, *project)
		if err != nil {
			return a.fail("plan repeat", err)
		}
		in.ProjectID = p.ID
	}
	if *team != "" {
		me, err := c.Me()
		if err != nil {
			return a.fail("plan repeat", err)
		}
		if in.TeamID, err = a.resolveTeam(c, me, *team); err != nil {
			return a.fail("plan repeat", err)
		}
	}
	sum, err := c.RepeatPlan(in)
	if err != nil {
		return a.fail("plan repeat", err)
	}
	return a.printPlanSummary("plan repeat", sum, *asJSON, *dryRun)
}

func (a *App) printPlanSummary(cmd string, sum *client.PlanSummary, asJSON, dryRun bool) int {
	if asJSON {
		return a.writeJSON(sum)
	}
	verb := "filled"
	if dryRun {
		verb = "would fill"
	}
	fmt.Fprintf(a.Out, "%s %d, skipped %d\n", verb, sum.Filled, sum.Skipped)
	if len(sum.Cells) == 0 {
		return 0
	}
	w := newTable(a.Out)
	fmt.Fprintln(w, "WEEK\tUSER\tPROJECT\tHOURS")
	for _, cell := range sum.Cells {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", isoWeek(cell.WeekStart), cell.UserID, cell.ProjectID, cell.Hours)
	}
	_ = w.Flush()
	return 0
}

// parseWeekStart accepts YYYY-Www or any YYYY-MM-DD and returns that week's Monday.
func parseWeekStart(value string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return plan.WeekStart(t), nil
	}
	if year, week, ok := strings.Cut(value, "-W"); ok {
		y, err1 := strconv.Atoi(year)
		w, err2 := strconv.Atoi(week)
		if err1 == nil && err2 == nil && w >= 1 && w <= 53 {
			jan4 := time.Date(y, time.January, 4, 0, 0, 0, 0, time.UTC)
			week1 := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
			return week1.AddDate(0, 0, (w-1)*7), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid week %q; use 2026-W46 or 2026-11-09", value)
}

func isoWeek(value string) string {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

func personName(names map[uint]string, id uint) string {
	if name, ok := names[id]; ok {
		return name
	}
	return fmt.Sprintf("user #%d", id)
}
