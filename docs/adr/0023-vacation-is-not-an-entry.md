# A vacation is its own record, not an entry

Status: accepted, 2026-10-30.

## Context

The plan board compares intended hours with each person's weekly capacity, and capacity was a single number per person. Time off was out of scope, so the unallocated row was an upper bound: a lead could not tell a free week from a week the person was away.

The obvious shortcut is to log vacation as an entry against a project, or to store a future-dated plan cell. Both reuse the timeline, reports and project picker, and both need no new table.

## Decision

A **vacation** is a separate row: one user, an inclusive start and end date, the hours off on the first and last date, and an optional note. It is never an entry and never appears in a report, a timeline, a CSV export or an entry list.

Only Monday-to-Friday time counts. The first and last date take their own hours when they are working days (a partial day is allowed, above zero and at most `weekly_hours / 5`, in 0.1-hour steps); the working days between them take a full day. A week's capacity is the weekly hours minus those vacation hours, rounded to the nearest hour and never below zero. Weekends never count, and a weekend edge stores zero.

A vacation is written by its owner only. A lead reads their team members' vacations and an admin reads every vacation, but neither writes one for someone else. A new range that overlaps an existing one for the same person is refused with `409 vacation-overlap`; there is no edit path, so the owner deletes and re-adds. Changing a vacation is not an audit event because it is the user's own data, as with entries under [ADR-0013](./0013-audit-role-actions.md).

## Why

- A timekeeping record and an intention have opposite lifecycles. Entries are evidence and are exported; a vacation is a capacity input that a person may correct after the fact. One table would need a flag on every read path, and one missed filter would report time off as worked hours.
- Partial days matter at the edges: a Friday afternoon or a Monday morning off is common. Storing hours on the first and last date keeps the common case one row, while per-day rows would make a two-week vacation fourteen records to maintain.
- Rejecting overlaps keeps capacity unambiguous. Allowing them would need a rule for summing or taking the maximum hours on a shared date, and that rule would be invisible in the UI.
- Owner-only writes match entries: an admin can read another person's hours but cannot rewrite them. A lead reads vacations to plan around them, not to keep someone else's calendar.
- Keeping vacations out of reports preserves the meaning of logged hours. A report still answers "what happened"; the plan board answers "what is left to plan".

## Considered options

- **Vacation as entries against a vacation project** — rejected: every report, CSV and timeline read would mix time off with work, and a project would exist only to be filtered out.
- **Future-dated plan cells** — rejected: a plan is an intention and a vacation is time off; seeding or repeating a plan would copy vacation, and a plan cell has no note or date range.
- **A calendar-day absence** — rejected: Saturdays and Sundays are not working days here, so a two-week vacation would incorrectly remove a weekend's capacity.
- **Per-day hours for the whole range** — rejected: it makes a long vacation many rows and a half-day edit a table operation, while nearly every day is a full day.
- **Fractional days per range** — rejected: it cannot say which day is half, so a Friday afternoon looks the same as a Wednesday morning.
- **Lead or admin writes for a member** — rejected: time off is personal, and the existing entry rule already refuses to let a role rewrite someone else's own record.
- **Overlaps summed or maximised** — rejected: either rule makes capacity depend on an invisible tie-break and invites double entry.
- **Vacation in reports as a category** — rejected: it would make the report's denominator a mixture of worked and absent time, and the plan board already shows the capacity effect.

## Consequences

- New table `vacations`, with `start_hours` and `end_hours`; an additive `AutoMigrate` change.
- Vacations carry the same `workspace_id` tenancy as every other tenant row.
- The API is `GET/POST /vacations` and `DELETE /vacations/{id}`, mirrored by the `/vacations` page. There is no CLI or MCP tool in this change.
- The plan board response gains per-person, per-week `vacation_hours` alongside the reduced `capacity`.
- Deactivated users keep their vacations, like their entries. A vacation may be added or deleted after its dates pass.
- A lead's team vacation view is read-only and bounded by the teams they lead; an admin's is workspace-wide.
