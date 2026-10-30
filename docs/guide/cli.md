# CLI reference

`tstamps` talks to the server's HTTP API. It never opens the database. Every client command accepts `--url` and `--token`; settings resolve as flags, then `TSTAMPS_URL` / `TSTAMPS_TOKEN`, then the config file written by `tstamps login`.

The saved URL carries the workspace: `https://time.example.com/w/example.com`. Every command then reaches that workspace and no other. See [Workspaces](./workspaces.md).

The commands are identical on Linux, macOS and Windows, so the three tabs below repeat the same line. Installation and config paths are the parts that differ; they are on [Install the CLI](./install-cli.md).

Exit codes: `0` ok, `1` error, `2` usage. `tstamps help` lists every command; `tstamps version` prints the version.

## Time input

| Form | Meaning |
| --- | --- |
| `09:00` | A clock time. The date is `--date`, or today. |
| `2026-10-19 09:00` | Date and time. |
| `2026-10-19T09:00:00+11:00` | An exact RFC 3339 instant. |
| `1h30m` | A duration, accepted by `--to` and counted from the start. |

- Times without a date are read in the machine's timezone, not your profile zone.
- A `--to` clock time that is not after the start crosses midnight: `--from 23:00 --to 01:00` is two hours.
- New entries store the machine's IANA timezone. Use `tstamps edit --timezone` to correct the zone that decides the entry's day.
- `--date` accepts `today`, `yesterday` or `YYYY-MM-DD`.

Projects are matched by name (case-insensitive), then by id; two projects with the same name need an id. Teams are matched among your memberships first, then by id. Users are `me`, an id, or an email (email lookup needs an admin).

## Session and tokens

### login

Authenticates with the workspace and saves credentials to `~/.config/tstamps/config.json` (mode `0600`).

By default, `tstamps login` uses the **OAuth 2.1 Device Grant** (`RFC 8628`): it prints a short verification code, opens `/w/<slug>/auth/device` in your browser (or prints the URL when `--no-browser` is passed), and stores both the access token and rotating refresh token once approved. The CLI and `tstamps mcp` automatically rotate expired access tokens using the saved refresh token.

For non-interactive scripts, passing `--email` (and `--password`) logs in with a password and creates a personal token (`ts_...`) instead. Password login survives for API tokens only where SSO is not enforced; a workspace with SSO has no CLI password path.

::: code-group
```sh [Linux]
tstamps login --url https://time.example.com/w/example.com
tstamps login --url https://time.example.com/w/example.com --no-browser
```

```sh [macOS]
tstamps login --url https://time.example.com/w/example.com
tstamps login --url https://time.example.com/w/example.com --no-browser
```

```powershell [Windows]
tstamps login --url https://time.example.com/w/example.com
tstamps login --url https://time.example.com/w/example.com --no-browser
```
:::

Log in to one workspace by putting its slug in the URL:

```sh
tstamps login --url https://time.example.com/w/example.com
```

The config file keeps that URL. Logging in again with another workspace URL replaces it.

### whoami

Prints your name, email, admin flag and memberships with roles.

::: code-group
```sh [Linux]
tstamps whoami
```

```sh [macOS]
tstamps whoami
```

```powershell [Windows]
tstamps whoami
```
:::

### tokens

Tokens are shown once at creation. `tstamps tokens create agent` is how an MCP client or a script gets its own credential; revoke it with `tokens rm`.

::: code-group
```sh [Linux]
tstamps tokens create agent
tstamps tokens ls
tstamps tokens rm 4
```

```sh [macOS]
tstamps tokens create agent
tstamps tokens ls
tstamps tokens rm 4
```

```powershell [Windows]
tstamps tokens create agent
tstamps tokens ls
tstamps tokens rm 4
```
:::

A password change or reset revokes every live token for that user. Log in again to mint a new one.

## Logging time

### start

Starts the running entry. At most one can run; a second start prints where the current entry started and exits `1`.

::: code-group
```sh [Linux]
tstamps start Timeline --note "OAuth callback"
```

```sh [macOS]
tstamps start Timeline --note "OAuth callback"
```

```powershell [Windows]
tstamps start Timeline --note "OAuth callback"
```
:::

### stop

Stops the running entry at now. `--note` appends to the entry's note.

::: code-group
```sh [Linux]
tstamps stop
tstamps stop --note "wrapped up"
```

```sh [macOS]
tstamps stop
tstamps stop --note "wrapped up"
```

```powershell [Windows]
tstamps stop
tstamps stop --note "wrapped up"
```
:::

### add

Logs a finished entry. `--from` and `--to` are required; `--date` dates bare times, `--note` is free text.

::: code-group
```sh [Linux]
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps add Timeline --from 23:00 --to 01:00 --date 2026-10-19
tstamps add Timeline --from 09:00 --to 1h30m
```

```sh [macOS]
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps add Timeline --from 23:00 --to 01:00 --date 2026-10-19
tstamps add Timeline --from 09:00 --to 1h30m
```

```powershell [Windows]
tstamps add Timeline --from 09:00 --to 11:30 --note "drag, resize"
tstamps add Timeline --from 23:00 --to 01:00 --date 2026-10-19
tstamps add Timeline --from 09:00 --to 1h30m
```
:::

### ls

Lists entries, including the running one. `--from` and `--to` are dates (`from` inclusive, `to` exclusive); the default range is the current week. `--user me|ID|email`, `--project NAME|ID`, `--json`.

::: code-group
```sh [Linux]
tstamps ls
tstamps ls --from 2026-10-19 --to 2026-10-26 --user me
tstamps ls --project Timeline --json
```

```sh [macOS]
tstamps ls
tstamps ls --from 2026-10-19 --to 2026-10-26 --user me
tstamps ls --project Timeline --json
```

```powershell [Windows]
tstamps ls
tstamps ls --from 2026-10-19 --to 2026-10-26 --user me
tstamps ls --project Timeline --json
```
:::

### edit

Changes an entry by id. Every flag is optional, but at least one is required. `--to` as a duration needs `--from`.

::: code-group
```sh [Linux]
tstamps edit 42 --from 09:15 --to 11:45
tstamps edit 42 --note "retro fix"
tstamps edit 42 --timezone Australia/Sydney
```

```sh [macOS]
tstamps edit 42 --from 09:15 --to 11:45
tstamps edit 42 --note "retro fix"
tstamps edit 42 --timezone Australia/Sydney
```

```powershell [Windows]
tstamps edit 42 --from 09:15 --to 11:45
tstamps edit 42 --note "retro fix"
tstamps edit 42 --timezone Australia/Sydney
```
:::

### rm

Soft-deletes an entry. It leaves every list and report; an admin can restore it.

::: code-group
```sh [Linux]
tstamps rm 42
```

```sh [macOS]
tstamps rm 42
```

```powershell [Windows]
tstamps rm 42
```
:::

## Projects and teams

`projects --all` includes archived projects and needs an admin. `projects add` needs the team you lead, or an admin.

::: code-group
```sh [Linux]
tstamps projects
tstamps projects add Timeline --team Platform --color "#7a5a8f"
tstamps projects archive 7
tstamps teams
tstamps teams members Platform
```

```sh [macOS]
tstamps projects
tstamps projects add Timeline --team Platform --color "#7a5a8f"
tstamps projects archive 7
tstamps teams
tstamps teams members Platform
```

```powershell [Windows]
tstamps projects
tstamps projects add Timeline --team Platform --color "#7a5a8f"
tstamps projects archive 7
tstamps teams
tstamps teams members Platform
```
:::

## report

Totals per bucket, person, team and project. Defaults: `--by week`; the range is the current week. `--by day|week|month|none`, `--team`, `--project`, `--user`, `--json`, `--csv`.

::: code-group
```sh [Linux]
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
tstamps report --from 2026-10-19 --to 2026-10-26 --by day --team Platform
tstamps report --from 2026-10-19 --to 2026-10-26 --json
tstamps report --from 2026-10-19 --to 2026-10-26 --csv > week.csv
```

```sh [macOS]
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
tstamps report --from 2026-10-19 --to 2026-10-26 --by day --team Platform
tstamps report --from 2026-10-19 --to 2026-10-26 --json
tstamps report --from 2026-10-19 --to 2026-10-26 --csv > week.csv
```

```powershell [Windows]
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
tstamps report --from 2026-10-19 --to 2026-10-26 --by day --team Platform
tstamps report --from 2026-10-19 --to 2026-10-26 --json
tstamps report --from 2026-10-19 --to 2026-10-26 --csv | Set-Content -Encoding utf8 week.csv
```
:::

`--json` and `--csv` are mutually exclusive. `--by none` returns one row for the whole range.

## plan

Planned hours by person, project and ISO week, against each person's vacation-adjusted weekly capacity. With no dates the range is next month. `--team`, `--user`, `--json`.

`plan set <person> <project> --week 2026-W46 --hours 20` writes one cell; `--hours 0` clears it. The week may be given as an ISO week or any date in it. `plan seed --from 2026-10 --into 2026-11` fills a month from the previous month's logged hours, and `plan repeat --week 2026-W46 --weeks 4` copies one week forward. Both fill empty cells only, take `--dry-run`, and print how many cells they filled and skipped.

A lead may plan people of their teams onto their teams' projects; an admin may plan anything. A week that has ended refuses writes. See [Plan](./plan.md). Vacations are added and read in the web UI or REST API, not from the CLI.

::: code-group
```sh [Linux]
tstamps plan --from 2026-11-01 --to 2026-12-01 --team Platform
tstamps plan set ana@example.com Timeline --week 2026-W46 --hours 20
tstamps plan rm ana@example.com Timeline --week 2026-W46
tstamps plan seed --from 2026-10 --into 2026-11 --dry-run
tstamps plan repeat --week 2026-W46 --weeks 4
```

```sh [macOS]
tstamps plan --from 2026-11-01 --to 2026-12-01 --team Platform
tstamps plan set ana@example.com Timeline --week 2026-W46 --hours 20
tstamps plan rm ana@example.com Timeline --week 2026-W46
tstamps plan seed --from 2026-10 --into 2026-11 --dry-run
tstamps plan repeat --week 2026-W46 --weeks 4
```

```powershell [Windows]
tstamps plan --from 2026-11-01 --to 2026-12-01 --team Platform
tstamps plan set ana@example.com Timeline --week 2026-W46 --hours 20
tstamps plan rm ana@example.com Timeline --week 2026-W46
tstamps plan seed --from 2026-10 --into 2026-11 --dry-run
tstamps plan repeat --week 2026-W46 --weeks 4
```
:::

## audit

The workspace's audit log, newest first; admin only. `--from` and `--to` are dates (`from` inclusive, `to` exclusive). `--actor EMAIL`, `--action NAME`, `--target-type TYPE`, `--target-id N`, `--limit N` (default 200, at most 1000), `--json`.

::: code-group
```sh [Linux]
tstamps audit --action project.archive --from 2026-10-19 --to 2026-10-26
```

```sh [macOS]
tstamps audit --action project.archive --from 2026-10-19 --to 2026-10-26
```

```powershell [Windows]
tstamps audit --action project.archive --from 2026-10-19 --to 2026-10-26
```
:::

See [Admin console](./admin.md#audit-log) for what an event records and what it does not.

## mcp

Runs the MCP server over stdio with the same config and token as the CLI. It blocks until the client closes the stream. See [MCP](./mcp.md).

::: code-group
```sh [Linux]
tstamps mcp
```

```sh [macOS]
tstamps mcp
```

```powershell [Windows]
tstamps mcp
```
:::

::: tip Common errors
- `no server URL` — run `tstamps login --url ...` or set `TSTAMPS_URL`.
- `404` — check the `/w/{slug}` part of the URL; an unknown slug selects no workspace.
- `already running since 09:41` — stop the running entry first; `tstamps stop`.
- `invalid timezone` / `422` on create — pass an IANA name such as `Europe/Berlin`, not a UTC offset.
- `password-change-required` — change the admin-issued initial password in the web UI before logging in.
- `401` after a password change — the reset revoked your token; log in again.
:::
