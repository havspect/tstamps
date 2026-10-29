# A plan is its own row, not a future-dated entry

Status: accepted, 2026-10-29.

## Context

Capacity planning asks what a team's hours will be spent on next month. Everything the schema holds is retrospective: an entry has started, and a report sums hours already logged. There was no way to say "Ana will spend 20 hours on Billing in W46".

The obvious shortcut is to write entries with future dates and mark them planned. It reuses the timeline, the report and the project picker, and it needs no new table.

## Decision

A **plan** is a separate row: one user, one project, one ISO week, a number of hours. It is never an entry and never appears in a report, a timeline, a CSV export or an entry list. Capacity is one number on the user, hours per week, defaulting to a full workweek.

A plan cell is one `(user, project, week)` with `0–168` hours. Zero hours deletes the cell. A week that has ended is frozen and refuses writes. Writing a plan is a role-based action and is audited; a member reads only their own plan.

## Why

- A timekeeping record and an intention have opposite lifecycles. Entries are evidence and are soft-deleted, restored and exported; plans are guesses that get rewritten weekly, and seeding them from last month is a normal operation. One table would need a flag on every read path — reports, CSV, timeline, MCP — and one missed filter would turn a guess into billed hours.
- Logged hours must keep meaning "what happened". Future-dated entries would make every report a mixture whose denominator nobody can name, and the five-minute future grace already says entries are about the past.
- A week is the unit of intent. Entries are minute-precise; plans are coarse on purpose, which is why a day-level block on the timeline was rejected.
- Separating the tables keeps the permission story small: report reads stay as they are, and planning gets two actions of its own.

## Considered options

- **Future-dated entries with `planned=true`** — rejected: every read path must exclude them, the 5-minute future rule must be relaxed, and one forgotten filter reports planned hours as real.
- **A projection from history with no stored plan** — rejected: it cannot express intent ("we start project Z in November"), and it silently staffs the next month off the last one's accidents. Kept as the one-click seed instead.
- **Blocks on the timeline for the future** — rejected: it invents hour-precise plans nobody maintains and puts plan data into the gesture surface that owns logged time.
- **A lifecycle with reminders and budgets** — rejected: a plan is a number a human looks at; consequences need delivery channels and false-alarm stories, and a different product.

## Consequences

- New table `plans`, plus `users.weekly_hours` (default 40). Both are additive `AutoMigrate` changes.
- Plan rows carry the same `workspace_id` tenancy as every other tenant row.
- The API is `/plan` (read, write, seed, repeat), mirrored by `tstamps plan` and four MCP tools.
- Archived projects keep their plan cells, visible and labelled, and cannot take new ones. Deactivated users lose their row unless they still have cells in the range.
- Over-capacity is allowed and flagged; the board shows a crunch rather than preventing a lead from entering a number.
- Bulk writes (`seed`, `repeat`) fill only empty cells and report what they skipped, and accept a dry run.
