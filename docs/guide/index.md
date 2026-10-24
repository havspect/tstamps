# Tstamps

Team time tracking for one company. Log hours by dragging on a timeline; whoever needs the numbers reads a report.

Tstamps is one `tstamps-server` process holding the SQLite database, the web timeline and the REST API, plus a small `tstamps` CLI that each person installs and points at that server. There is no hosted service, no per-seat subscription and no second system to reconcile.

## How the pieces fit

- **Workspace** — one installation, one company. It holds every user, team and project.
- **Team** — a group of users that owns projects. A user can be in several teams.
- **Lead** — a membership role: creates that team's projects and reads every hour booked on them.
- **Member** — logs their own hours and reads their own totals.
- **Admin** — manages users, teams, memberships, and restores archived projects or deleted entries.
- **Entry** — one continuous stretch of work on one project, with a start, an end and an optional note. A **running entry** has a start but no end yet; there is at most one per user.

| Action | Member | Lead | Admin |
| --- | --- | --- | --- |
| Log, edit and delete own entries | yes | yes | yes |
| Read own hours | yes | yes | yes |
| Read hours on their teams' projects | no | yes | yes |
| Create and archive projects | no | their teams | all |
| Manage teams and memberships | no | no | yes |
| Create and deactivate users, reset passwords | no | no | yes |
| Restore deleted entries and archived projects | no | no | yes |

Vocabulary is [GLOSSARY.md](https://github.com/havspect/tstamps/blob/main/GLOSSARY.md); what v1 does and does not do is [docs/SPEC.md](https://github.com/havspect/tstamps/blob/main/docs/SPEC.md).

## Quick start

If your workspace is already running, install the [CLI](./install-cli.md) and log in. The commands are the same on all three platforms; the tabs exist because the manual shows every command for every platform.

::: code-group
```sh [Linux]
tstamps login --url https://time.example.com
tstamps whoami
tstamps start Timeline --note "OAuth callback"
tstamps stop
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps ls
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
```

```sh [macOS]
tstamps login --url https://time.example.com
tstamps whoami
tstamps start Timeline --note "OAuth callback"
tstamps stop
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps ls
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
```

```powershell [Windows]
tstamps login --url https://time.example.com
tstamps whoami
tstamps start Timeline --note "OAuth callback"
tstamps stop
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps ls
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
```
:::

Or open the timeline in the browser after signing in and drag on a lane. The [timeline](./timeline.md) and the [CLI](./cli.md) write the same entries.

## What it deliberately leaves out

Approvals, leave and holidays, invoicing and rates, reminders, a mobile app, screenshots, Jira/Linear sync, realtime co-editing, project budgets, a team-wide timeline, SSO, an audit log, i18n, and drafting entries from coding-agent history.

If you need any of those, choose something else. [The spec](https://github.com/havspect/tstamps/blob/main/docs/SPEC.md) is the full list.

## Next

- [Install the CLI](./install-cli.md) on Linux, macOS or Windows.
- [Run the server](./server.md) on a Linux host.
- [Back up and upgrade](./backup-upgrade.md) without losing hours.
