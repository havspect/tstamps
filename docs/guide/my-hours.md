# My hours

Open `/w/{slug}/my-hours` after signing in. It is yours alone: whatever your role, it shows only the entries you logged. For the team-wide picture, [Reports](./reports.md) is the page.

The chart is one bar per bucket, stacked by project in the project's own color, so the shape of a week is one glance:

- **Day** — one bar per day of the range.
- **Week** — one bar per ISO week.
- **Month** — one bar per calendar month.

The **Range** preset picks the granularity that goes with it: this week and last week read as days, this month as weeks, this year as months. A custom range keeps whichever **Day | Week | Month** switch you set.

Hover a bar for the per-project hours behind it, and click a project in the legend to hide it.

The table below carries the same numbers as the chart, one row per project and one column per bucket, with a total on each. It is also the chart's accessible copy, so a screen reader reads the table rather than the canvas. The three figures above it are the total logged, how many projects that covers, and which project took the largest share.

**Download CSV** exports the detail behind the counts: one row per entry per local day, with the note.

## What counts

The same rules as [Reports](./reports.md#what-counts): archived projects are included and labelled, deleted entries are excluded, an entry crossing local midnight is split at that midnight, a still-running entry counts up to now, and hours are bucketed by the entry's own timezone.

## Rows you can read

The API scopes a report to the caller and you cannot widen it from the page:

- An **admin** reads every entry in the workspace.
- A **lead** reads every entry on the projects of the teams they lead, plus their own entries anywhere.
- A **member** reads only their own entries.

A filter narrows that scope, never widens it: sending a `team_id` or `user_id` outside what you may read returns only what you may read.

The same totals are available without a browser:

::: code-group
```sh [Linux]
tstamps report --from 2026-10-19 --to 2026-10-26 --user me
```

```sh [macOS]
tstamps report --from 2026-10-19 --to 2026-10-26 --user me
```

```powershell [Windows]
tstamps report --from 2026-10-19 --to 2026-10-26 --user me
```
:::

See the [CLI reference](./cli.md#report).
