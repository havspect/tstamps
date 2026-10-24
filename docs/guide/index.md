# Tstamps

Team time tracking. Log hours by dragging on a timeline; whoever needs the numbers reads a report.

Tstamps is one `tstamps-server` process holding the SQLite database — or one per hosted workspace — plus the web timeline and the REST API, and a small `tstamps` CLI that each person installs and points at that server. Run it for one company, or host several companies as separate workspaces on the same server. There is no managed service, no per-seat subscription and no second system to reconcile.

## How the pieces fit

- **Workspace** — one company's data: every user, team, project and entry, in its own SQLite file. A server runs either one workspace (`--db`) or several (`--workspace-dir`), each served under its own `/w/{slug}` URL.
- **Team** — a group of users that owns projects. A user can be in several teams.
- **Lead** — a membership role: creates that team's projects and reads every hour booked on them.
- **Member** — logs their own hours and reads their own totals.
- **Admin** — manages users, teams, memberships, and restores archived projects or deleted entries, within one workspace. There is no server-wide admin.
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

If your workspace is already running, install the [CLI](./install-cli.md) and log in. On an operator-hosted server the URL names your workspace, for example `https://time.example.com/w/example.com`; see [Hosted workspaces](./workspaces.md). The commands below are the same in both shapes. They are also the same on all three platforms; the tabs exist because the manual shows every command for every platform.

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

Approvals, leave and holidays, invoicing and rates, reminders, a mobile app, screenshots, Jira/Linear sync, realtime co-editing, project budgets, a team-wide timeline, SSO, an audit log, and i18n. Drafting entries from coding-agent history is not in the server either; the [agent skill](./agents/index.md) does that on the client.

If you need any of those, choose something else. [The spec](https://github.com/havspect/tstamps/blob/main/docs/SPEC.md) is the full list.

## Next

- [Install the CLI](./install-cli.md) on Linux, macOS or Windows.
- [Give an agent the skill](./agents/index.md) for Amp, Claude, Codex, Langdock or OpenCode.
- [Run the server](./server.md) on a Linux host.
- [Host several companies](./workspaces.md) as separate workspaces.
- [Back up and upgrade](./backup-upgrade.md) without losing hours.
