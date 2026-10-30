# Vacations

Open **Vacations** in the top bar. Every user records their own time off; nobody records leave for someone else. A lead can switch the person filter to a member of a team they lead, and an admin can read anyone, but those views are read-only.

A vacation is an inclusive range of dates. Pick the first day and the last day, enter the hours off on each edge, add an optional note and press **Add vacation**.

## Whole days and partial days

The first and last dates are entered in hours, so a Friday afternoon or a Monday morning off is a partial day. On a working day, each edge is above zero and at most one working day (`weekly_hours / 5`), in 0.1-hour steps; a weekend edge is 0. For a one-day vacation there is one hours field.

Working days between the edges are full days: each removes `weekly_hours / 5`. Saturdays and Sundays never count, so a range across a weekend changes nothing. An edge that falls on a weekend is fixed at 0 hours.

That week's capacity is the person's weekly hours minus the vacation hours, rounded to the nearest hour and never below zero. Five full working days remove the whole week. A 37-hour week's full day is 7.4 hours.

A new range may not overlap an existing one for the same person: the API answers `409 vacation-overlap`. Delete the old range and add it again with the right dates or hours.

## On the plan board

The plan board marks a vacation week with `vac` and shows the reduced capacity. Planning over a vacation is allowed and flagged like any other over-capacity week.

A vacation is not logged time. It never appears in reports, CSV exports or on the timeline, and changing one is not an audit event. The CLI and MCP tools do not expose vacations yet; use this page or the REST API.
