# Reports

Open `/w/{slug}/reports` after signing in. Reports are read-only.

- A **lead** reads every hour booked on projects owned by a team they lead, whoever logged it, plus their own entries.
- An **admin** reads everything.
- A **member** has no report page; `tstamps report` returns their own hours.

![The reports table: projects as rows, days as columns, total on the right](/screenshots/reports.webp)

## Read the table

Projects are rows, active first and biggest first within each group. Each column is one day, one ISO week or one calendar month, depending on the granularity. The right column totals each project, and the bottom row totals the workspace.

Click a project to unfold one line per person.

The controls above the table:

- **Day | Week | Month** — the bucket size.
- **Range** — this week, last week, this month or a custom range. Choosing a preset also picks the matching granularity.
- **Team** — only shown when you can read more than one team.
- **Download CSV** — the detail behind the table.

## What counts

- Reports include hours on archived projects, labelled as archived.
- Deleted entries are excluded. An admin can restore them under **Admin → Entries**.
- An entry that crosses local midnight is split into two rows at that midnight.
- Hours are bucketed by the entry's own timezone, not the viewer's and not the user's profile zone.
- A 12-hour entry crossing local midnight still sums to 12 hours across a DST change.

## CSV

The CSV has one row per entry per local day:

```csv
date,iso_week,user,email,team,project,timezone,start,end,seconds,note
2026-10-19,2026-W43,Ada Lovelace,ada@example.com,Platform,Timeline,Australia/Sydney,2026-10-19T09:00:00+11:00,2026-10-19T11:30:00+11:00,9000,"drag, resize, snap"
```

The same export is available without a browser:

::: code-group
```sh [Linux]
tstamps report --from 2026-10-19 --to 2026-10-26 --csv > week.csv
```

```sh [macOS]
tstamps report --from 2026-10-19 --to 2026-10-26 --csv > week.csv
```

```powershell [Windows]
tstamps report --from 2026-10-19 --to 2026-10-26 --csv | Set-Content -Encoding utf8 week.csv
```
:::

`--by day|week|month|none` changes the aggregation, and `--team`, `--project` and `--user` filter it. See the [CLI reference](./cli.md#report).
