package cli

import (
	"fmt"

	"github.com/havspect/tstamps/internal/client"
)

func (a *App) audit(args []string) int {
	fs := newFlags("audit")
	urlFlag, tokenFlag := clientFlags(fs)
	from := fs.String("from", "", "inclusive start date, YYYY-MM-DD")
	to := fs.String("to", "", "exclusive end date, YYYY-MM-DD")
	actor := fs.String("actor", "", "actor email or id")
	action := fs.String("action", "", "action name, e.g. user.update")
	targetType := fs.String("target-type", "", "target type, e.g. project")
	targetID := fs.String("target-id", "", "target id")
	limit := fs.Int("limit", 0, "maximum events; default 200, at most 1000")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("audit", err)
	}
	if code, ok := a.checkNoArgs("audit", fs); !ok {
		return code
	}

	c, err := a.connect(*urlFlag, *tokenFlag)
	if err != nil {
		return a.fail("audit", err)
	}
	q := client.AuditQuery{
		From: *from, To: *to,
		Action: *action, TargetType: *targetType, Limit: *limit,
	}
	if *actor != "" {
		if q.ActorUserID, err = a.resolveUser(c, *actor); err != nil {
			return a.fail("audit", err)
		}
	}
	if *targetID != "" {
		if q.TargetID, err = parseID(*targetID); err != nil {
			return a.fail("audit", err)
		}
	}

	events, err := c.ListAudit(q)
	if err != nil {
		return a.fail("audit", err)
	}
	if *asJSON {
		return a.writeJSON(events)
	}

	w := newTable(a.Out)
	fmt.Fprintln(w, "AT\tACTOR\tROLE\tACTION\tTARGET")
	for _, e := range events {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s/%d\n",
			e.At.Local().Format(dateLayout+" 15:04"), e.ActorEmail, e.ActorRole,
			e.Action, e.TargetType, e.TargetID)
	}
	_ = w.Flush()
	return 0
}
