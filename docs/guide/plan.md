# Plan

Open `/w/{slug}/plan` after signing in. It is the forward-looking twin of Reports: how many hours each person will spend on which project, week by week, against how many hours they can work. Reports say what happened; Plan says what is intended.

![The plan board: one row per person, one column per ISO week, stacked project bars against capacity](/screenshots/plan.webp)

- A **lead** sees their own teams' people and reads and writes plans on their teams' projects. Load a person carries on another team's project appears as grey "other" hours, so the row does not lie, without naming the project.
- A cell may only name a project of the planned person's teams, so nobody is planned onto work their lead cannot see. An admin included: adding the person to the team comes first (ADR-0023).
- An **admin** sees and plans everything.
- A **member** sees their own row read-only.

A plan is never an entry. It does not appear in reports, CSV exports or the timeline (ADR-0022).

## Read the board

Choose **Week**, **Month** or **Quarter**; the range defaults to next month. Pick any day in the range and press **Show**. Each column is one ISO week, and a cell's hours are always for that week, whatever the range. A bar is coloured by project, using the same project colours as the timeline. The right column compares planned hours with the person's availability for the range — their capacity, adjusted by a per-week override and any vacation — and the bottom row shows how much availability is unplanned; every capacity figure on the board, the KPIs included, uses availability.

Every bar shares one scale: the largest availability among the people and weeks on screen, rounded up to the next 5 hours, named in the caption. A track is as wide as the person's availability that week and fills from the left, so a shorter track means less availability, not fewer hours; the value reads `20/32`, planned of available.

Switch **By person | By project** to read the same numbers the other way: which projects will consume the range. A project row expands (▸) to its team's members, and each member's week is the same editable cell as in the person view, on the same scale against that member's availability; project summary rows stay plain hour numbers. Archived projects and their members stay read-only.

## Set hours

Click a person's week, or a member's week under a project, to open the editor. From the person view, choose a project, type the hours and press **Set**; from the project view the project is fixed and the person is the row you clicked. Setting `0` removes that project from the week. A person may be planned for up to 168 hours a week; planning beyond their availability is allowed and marked, because a crunch is the thing you came to see.

In the person view, **Available this week** above the project hours sets that week's availability, `0–168`, without touching any project hours. Setting it back to the person's default clears the override, so a later capacity change on **Admin → Users** still applies; `0` means genuinely unavailable. The project view edits only project hours.

**Repeat 4 weeks** copies that person's week (or that person's hours on the clicked project) into the following four, and **Seed from last month** fills the whole month from the previous month's logged hours: each person's hours per project are split evenly across the month's weeks and rounded to whole hours.

Bulk actions fill empty cells only and tell you how many they skipped. Nothing is overwritten, so running a seed twice is harmless.

## Rules worth knowing

- A week that has ended, measured in the planned person's timezone, is frozen and refuses writes. Current and future weeks accept them.
- Archived projects keep their existing plan hours, labelled as archived, but cannot take new ones.
- A deactivated person keeps their row only while they still have plan hours in the range.
- Capacity is a number per person on **Admin → Users**, defaulting to 40 hours. A [vacation](./vacations.md) lowers it by the working time it covers, so the unallocated row already excludes it.
- A person's availability defaults to their capacity; only weeks that differ are stored, so changing capacity carries through every week without an override. An override applies before any vacation hours are taken off.
- Every cell a lead or admin writes is an audit event, one per cell, including the cells a seed or repeat fills; an availability override is audited the same way.

## Vacations

Every user records their own vacations under **Vacations** in the top bar. A vacation is an inclusive range of dates with hours on its first and last date; the working days between them are full days. Only Monday-to-Friday time counts, so a range across a weekend does not change the board. A week cell whose person is away is marked `vac` and shows the reduced capacity, and planning over it is allowed but flagged like any other crunch. A vacation is taken off the week's availability, after any override.

A lead can switch the **Vacations** page to a person in a team they lead and read the dates, hours and note. That view is read-only: only the owner writes a vacation.

A vacation is not logged time. It never appears in reports, CSV exports or the timeline.

## Without a browser

```sh
tstamps plan --from 2026-11-01 --to 2026-12-01 --team Platform
tstamps plan set ana@example.com Timeline --week 2026-W46 --hours 20
tstamps plan rm ana@example.com Timeline --week 2026-W46
tstamps plan seed --from 2026-10 --into 2026-11 --dry-run
tstamps plan repeat --week 2026-W46 --weeks 4 --dry-run
```

`--dry-run` on a bulk prints exactly what it would fill and writes nothing. `--json` on any of them prints the machine-readable shape. The capacity total in `tstamps plan` uses the same vacation-adjusted weeks as the board. Vacations are added in the web UI; the CLI and MCP tools do not expose them yet. Agents reach the same operations through the `get_plan`, `set_plan`, `seed_plan` and `repeat_plan` MCP tools. See the [CLI reference](./cli.md).
