# Plan

Open `/w/{slug}/plan` after signing in. It is the forward-looking twin of Reports: how many hours each person will spend on which project, week by week, against how many hours they can work. Reports say what happened; Plan says what is intended.

- A **lead** sees their own teams' people and reads and writes plans on their teams' projects. Load a person carries on another team's project appears as grey "other" hours, so the row does not lie, without naming the project.
- An **admin** sees and plans everything.
- A **member** sees their own row read-only.

A plan is never an entry. It does not appear in reports, CSV exports or the timeline (ADR-0022).

## Read the board

Choose **Week**, **Month** or **Quarter**; the range defaults to next month. Pick any day in the range and press **Show**. Each column is one ISO week, and a cell's hours are always for that week, whatever the range. A bar is coloured by project, using the same project colours as the timeline. The right column compares planned hours with the person's capacity for the range, and the bottom row shows how much capacity is unplanned.

Switch **By person | By project** to read the same numbers the other way: which projects will consume the range. A project row expands (▸) to its team's members, and each member's week is the same editable cell as in the person view. Archived projects and their members stay read-only.

## Set hours

Click a person's week, or a member's week under a project, to open the editor. From the person view, choose a project, type the hours and press **Set**; from the project view the project is fixed and the person is the row you clicked. Setting `0` removes that project from the week. A person may be planned for up to 168 hours a week; over capacity is allowed and marked, because a crunch is the thing you came to see.

**Repeat 4 weeks** copies that person's week (or that person's hours on the clicked project) into the following four, and **Seed from last month** fills the whole month from the previous month's logged hours: each person's hours per project are split evenly across the month's weeks and rounded to whole hours.

Bulk actions fill empty cells only and tell you how many they skipped. Nothing is overwritten, so running a seed twice is harmless.

## Rules worth knowing

- A week that has ended, measured in the planned person's timezone, is frozen and refuses writes. Current and future weeks accept them.
- Archived projects keep their existing plan hours, labelled as archived, but cannot take new ones.
- A deactivated person keeps their row only while they still have plan hours in the range.
- Capacity is a number per person on **Admin → Users**, defaulting to 40 hours. Leave and holidays are not tracked, so treat the unallocated row as an upper bound.
- Every cell a lead or admin writes is an audit event, one per cell, including the cells a seed or repeat fills.

## Without a browser

```sh
tstamps plan --from 2026-11-01 --to 2026-12-01 --team Platform
tstamps plan set ana@example.com Timeline --week 2026-W46 --hours 20
tstamps plan rm ana@example.com Timeline --week 2026-W46
tstamps plan seed --from 2026-10 --into 2026-11 --dry-run
tstamps plan repeat --week 2026-W46 --weeks 4 --dry-run
```

`--dry-run` on a bulk prints exactly what it would fill and writes nothing. `--json` on any of them prints the machine-readable shape. Agents reach the same operations through the `get_plan`, `set_plan`, `seed_plan` and `repeat_plan` MCP tools. See the [CLI reference](./cli.md).
