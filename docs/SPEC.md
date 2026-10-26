# Tstamps — specification

A team time tracker in the spirit of timeTagger, minus the one-person focus: a personal infinite timeline for logging, a plain table for whoever needs the numbers. One Go binary, one SQLite database holding one or more workspaces.

Vocabulary lives in [GLOSSARY.md](../GLOSSARY.md). Decisions that are expensive to reverse live in [docs/adr](./adr/). The paid hosted service, SSO, remote MCP and mobile app are specified in [SPEC-v2.md](./SPEC-v2.md).

## Scope

In v1:

- Users log hours on a personal timeline, with one running entry at a time.
- Leads create projects for their teams and read hours logged on those projects.
- Admins manage users, teams, memberships, and restore deleted data.
- Reports per person / project / day or week, plus CSV.
- A REST API, a CLI, and an MCP server (stdio), all over the same HTTP API.
- An operator may host several workspaces on one server, each with its own slug and rows scoped by `workspace_id` ([ADR-0019](./adr/0019-workspaces-share-one-sqlite-file.md)).
- A server may offer self-service signup: an email owner verifies the address, then creates a workspace and becomes its first admin ([ADR-0014](./adr/0014-self-service-workspace-signup.md)).

Out of v1, deliberately: approvals, leave and holidays, invoicing and rates, reminders and notifications, mobile app, screenshots, Jira/Linear sync, realtime multi-user editing, project budgets, a team-wide timeline, SSO, i18n, drafting entries from coding-agent session history, billing, and workspace suspension or deletion workflows.

## Roles

A user has exactly one of two global states — admin or not — plus zero or more team memberships, each with role `lead` or `member`.

| Action | Member | Lead | Admin |
| --- | --- | --- | --- |
| Log / edit / delete own entries | yes | yes | yes |
| Read own hours | yes | yes | yes |
| Read hours on their teams' projects | no | yes | yes |
| Create / edit / archive projects | no | their teams | all |
| Manage teams and memberships | no | no | yes |
| Create / deactivate users, reset passwords | no | no | yes |
| Restore deleted entries / archived projects | no | no | yes |
| Any project picker | all non-archived | all non-archived | all |

A lead's report scope is *entries booked on projects owned by a team they lead*, whoever logged them, plus their own entries. Admin scope is everything.

## Domain model

```
workspaces(id, slug)
pending_signups(token_hash, email, created_at)
users(id, email, name, password_hash, timezone, is_admin, active, must_change_password, created_at, updated_at)
teams(id, name, created_at, updated_at, archived_at)
team_memberships(id, team_id, user_id, role, created_at)         unique(team_id, user_id)
projects(id, team_id, name, color, created_at, updated_at, archived_at)
entries(id, user_id, project_id, start_at, end_at, timezone, note, created_at, updated_at, deleted_at)
sessions(id, user_id, token_hash, expires_at, created_at)
api_tokens(id, user_id, name, token_hash, created_at, last_used_at, revoked_at)
audit_events(id, at, actor_user_id, actor_token_id, actor_role, action, target_type, target_id, changes)
```

Every tenant row (`users` through `audit_events`) also carries `workspace_id`, and unique keys such as email are composite with it. `workspaces` and `pending_signups` are server-wide; a pending signup belongs to no workspace until its link is submitted.

Rules the store enforces, with tests:

- A user has at most one entry with `end_at IS NULL` and `deleted_at IS NULL` (partial unique index).
- `end_at > start_at`, and a finished entry is no longer than 24 hours. Stopping a running entry that would exceed 24 hours returns `422 entry-too-long` and leaves it running, so the user picks an end time instead of losing hours.
- Start and end may not be more than 5 minutes in the future.
- Overlapping entries are allowed. The timeline marks them; the API does not reject them, because parallel work is real.
- Entries may span midnight; reports split them at the entry's local midnight.
- Archived projects reject new entries but keep existing ones. Archiving a team archives its projects in the same step; the membership rows stay but grant nothing. A project of an archived team cannot be restored (`422 invalid-input`) until the team is live again.
- A project's team cannot change once the project has entries (`422 project-has-entries`); while empty it can.
- Deactivated users cannot log in or use tokens; their hours stay. Deactivating a user stops their running entry at that instant, with the note `stopped on deactivation`.
- A user created or reset by an admin carries `must_change_password`; until it clears, only `/me`, `/me/password` and `/auth/logout` work (`403 password-change-required`).
- A password change or reset ends every other login of that user in the same transaction: the sessions are deleted and every live token is revoked. A self-change keeps only the session that made it, so a token client logs in again.
- The last active admin cannot be demoted or deactivated (`422 last-admin`).
- An audit event commits in the same transaction as the mutation it describes, never after it ([ADR-0013](./adr/0013-audit-role-actions.md)).
- Deleting an entry sets `deleted_at`; restoring clears it. No hard deletes anywhere.

Email comparison is case-insensitive; emails are stored lowercased.

## Time and reporting

All instants are UTC. Each entry stores the IANA timezone it was logged in; days and ISO weeks are computed in that entry's timezone, not the viewer's and not the user's profile zone ([ADR-0007](./adr/0007-entry-timezones-decide-buckets.md)). A user also has a timezone, used only when a range query has no dates and for display.

- A report takes `from` and `to` as plain dates, `from` inclusive, `to` exclusive.
- Each query fetches entries overlapping `[from−14h, to+14h)` in UTC, then buckets and splits them in Go.
- Buckets are `week` (ISO, Monday start), `day` (the entry's local date), `month` (the entry's local calendar month), or `none` (whole range).
- A 12-hour entry crossing local midnight becomes two rows, and their seconds still sum to 12h across DST shifts.
- An entry's `timezone` is an IANA name the server knows; creating one without it (including `POST /timer/start`) is a `422`. `PATCH /entries/{id}` may correct it.
- Reports include hours on archived projects, labelled as archived. They exclude deleted entries.

CSV export is the detail behind the table: one row per entry per local day.

```csv
date,iso_week,user,email,team,project,timezone,start,end,seconds,note
2026-10-19,2026-W43,Ada Lovelace,ada@example.com,Platform,Timeline,Australia/Sydney,2026-10-19T09:00:00+11:00,2026-10-19T11:30:00+11:00,9000,"drag, resize, snap"
```

## HTTP API

`/api/v1`, JSON in and out, generated from the Go handlers by Huma. Every workspace route is under `/w/{slug}`, so the paths below are relative to that prefix. The OpenAPI 3.1 document is served at `/w/{slug}/api/v1/openapi.json` and drives both the web client's types ([ADR-0006](./adr/0006-generated-api-contract.md)) and the API reference at `/w/{slug}/api/v1/docs`. Huma validates bodies and query parameters from struct tags (`422` for malformed input); authentication failures stay `401`/`403` from middleware.

Errors are RFC 9457 problem+json. The stable machine-readable code lives in the `type` URI, not in prose:

```json
{"type":"urn:tstamps:error:timer-already-running","title":"Conflict","status":409,"detail":"Nadia already has a running entry.","instance":"/api/v1/timer/start"}
```

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/auth/login` | email + password, sets session cookie |
| POST | `/auth/logout` | clears session |
| GET | `/me` | current user, memberships, admin flag |
| POST | `/me/password` | change own password |
| GET/POST | `/me/tokens` | list / create personal API tokens |
| DELETE | `/me/tokens/{id}` | revoke |
| GET/POST | `/users` | admin: list / create |
| PATCH | `/users/{id}` | admin: name, timezone, admin flag, active |
| POST | `/users/{id}/password` | admin: reset |
| GET/POST | `/teams` | list / create (admin) |
| PATCH/DELETE | `/teams/{id}` | rename / archive, which archives its projects (admin) |
| PUT | `/teams/{id}/members` | admin: replace membership list `[{user_id, role}]` |
| GET/POST | `/projects` | list / create (lead for own team, admin) |
| PATCH | `/projects/{id}` | name, color, archive; team only while it has no entries |
| POST | `/projects/{id}/restore` | admin |
| GET | `/entries?from&to[&user_id]` | range query, default current week, max 366 days |
| POST | `/entries` | create a finished entry; `timezone` required |
| PATCH | `/entries/{id}` | move, retime, re-project, re-note, correct `timezone` |
| DELETE | `/entries/{id}` | soft delete |
| POST | `/entries/{id}/restore` | admin |
| GET | `/timer` | current running entry or null |
| POST | `/timer/start` | `{project_id, note, timezone}`; 409 with the running entry if one exists |
| POST | `/timer/stop` | ends at now, optional note |
| GET | `/reports?from&to&by=week\|day\|month\|none[&team_id][&user_id][&project_id]` | aggregated seconds |
| GET | `/reports.csv` | same filters, entry-per-day rows |
| GET | `/audit?from&to[&actor_user_id][&action][&target_type][&target_id][&limit]` | admin: role-based actions, newest first |
| GET | `/healthz` | database reachability + schema version |
| GET | `/openapi.json` | OpenAPI 3.1 document, generated from handlers |
| GET | `/docs` | browsable API reference |

The root `/healthz`, outside the workspace prefix, only pings the database.

Four more routes answer at the root, before any workspace is selected, so they are not in the per-workspace OpenAPI document:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/workspace` | `{slug}` when exactly one workspace exists, otherwise `{"slug":""}`; lets the SPA skip the prompt |
| POST | `/api/v1/signup` | `{email}`: send the verification link; `202` for a well-formed address |
| GET | `/api/v1/signup/verify?token=...` | the address the link was sent to; does not consume it |
| POST | `/api/v1/signup/verify` | `{token, slug, name, password}`: create the workspace and its first admin, set a session cookie |

They exist only when self-service signup is configured; otherwise each answers `503 urn:tstamps:error:unavailable`. Sending is limited to five per hour per IP (`429 urn:tstamps:error:rate-limited`), because that step mails an arbitrary address. A token is single use and never expires; an unknown or used one is `404 urn:tstamps:error:invalid-token`. Both POSTs require a same-origin `Origin`. The address is lowercased before it is stored or sent. A mail failure is `502 urn:tstamps:error:email-failed` and leaves no usable link.

Authentication: `POST /auth/login` returns an opaque session token in an `HttpOnly`, `SameSite=Lax` cookie, 30-day fixed expiry, stored hashed. The cookie is named `__Host-tstamps_session` and marked `Secure` when the request arrives over TLS or with `X-Forwarded-Proto: https`; over plain http it is `tstamps_session` without `Secure`, because browsers reject the `__Host-` prefix there. Production therefore sits behind a TLS-terminating proxy, bound to localhost. Every other request may instead send `Authorization: Bearer ts_...`. Tokens are shown once at creation, stored as SHA-256, and act as the owning user. Login is rate limited per IP in memory. A user with `must_change_password` can authenticate but is confined to the three endpoints listed in the rules above.

The selected workspace is resolved from `/w/{slug}` before authentication, and credentials are only looked up within that workspace. `GET /login` redirects to `/w/{slug}/login` when the database holds exactly one workspace, and shows the slug prompt when it holds more. A browser holds one workspace login per server: logging into another workspace replaces the shared session cookie. CLI and MCP clients carry the workspace in their configured base URL.

Mutations over the cookie require a same-origin `Origin` header. There is no CORS: the SPA is served from the same origin.

Every workspace API request writes one structured log line to stderr (method, path without query, status, duration, bytes, remote IP, user id when authenticated). The host-level signup requests and the SPA are not logged. `TSTAMPS_LOG_LEVEL` or `--log-level` sets the minimum level; the default is `info`.

Actions that spend authority over other people's data are audited separately; see [Audit log](#audit-log).

## Audit log

An audit event records a change made by a user whose authority over the target comes from a role — an admin, or a lead acting on their team's projects — never from ownership. A user editing their own entry writes none. The terms are in [GLOSSARY.md](../GLOSSARY.md).

- **Recorded:** user create, update, password reset; team create, rename, archive; membership replace; project create, update, restore, archive; entry restore; and the `entry.stop` that deactivating a user causes.
- **Not recorded:** anything a user does to their own data — logging, editing, deleting entries, timer start and stop, own password change, own tokens — and login traffic, which stays in the stderr request line.
- `changes` holds the audited fields before and after. Password and token hashes never appear; a password reset records that it happened.
- Cascades write their own events: a team archive writes one `project.archive` per archived project, and a deactivation writes one `entry.stop` for the running entry it ends.
- Events are append-only: no update or delete path and no retention job. They live in the workspace's rows, so backup and restore carry them. The log resists the application, not an operator who owns the file.
- Reading is admin-only, newest first, default 200 rows, maximum 1000, over `GET /audit` and `tstamps audit`. There is no web page and no MCP tool.
- No backfill: the log starts empty on the day the binary that writes it ships.

## CLI

The server and the CLI are two binaries ([ADR-0008](./adr/0008-user-cli-and-server-are-two-binaries.md)). `tstamps-server` runs a workspace and owns the database; `tstamps` is the user CLI, shipped for linux, macOS and Windows, and talks to the API, never to the database. Config from `--url` / `--token`, then `TSTAMPS_URL` / `TSTAMPS_TOKEN`, then `~/.config/tstamps/config.json` (mode 0600). The CLI and the MCP server share `internal/client` and the API's Go structs, so neither needs generated code.

```
tstamps-server serve [--addr :8080] [--db ./tstamps.db]   # web UI + API for every workspace
tstamps-server workspace create example.com --db ./tstamps.db --email x --name y
tstamps-server backup [--db path] <destination-file>
tstamps-server openapi                                    # the OpenAPI document, for the web build
tstamps login --url https://tstamps.example.com/w/example.com  # password prompt, stores a personal token
tstamps whoami
tstamps start <project> [--note "..."]             # project matched by name, then id
tstamps stop [--note "..."]
tstamps add <project> --from 09:00 --to 11:30 [--date today] [--note "..."]
tstamps ls [--from 2026-10-19 --to 2026-10-26] [--user me|email] [--json]
tstamps edit <id> [--project ...] [--from ...] [--to ...] [--timezone ...] [--note ...]
tstamps rm <id>
tstamps projects [--all] | tstamps projects add <name> --team <team> | tstamps projects archive <id>
tstamps teams | tstamps teams members <team>
tstamps report --from 2026-10-19 --to 2026-10-26 [--by week|day|month] [--json|--csv]
tstamps tokens create <name> | tstamps tokens ls | tstamps tokens rm <id>
tstamps audit [--from 2026-10-01 --to 2026-11-01] [--actor <email>] [--action user.update] [--target-type project] [--target-id 7] [--limit 50] [--json]
tstamps mcp                                        # MCP server on stdio
tstamps version
```

Times without a date mean today in the machine's timezone. Durations are accepted as `1h30m`. Exit codes: 0 ok, 1 error, 2 usage.

## MCP

`tstamps mcp` speaks MCP over stdio and uses the same config and token as the CLI. Tools mirror the API, with read tools annotated read-only:

`whoami`, `list_projects`, `list_entries`, `start_timer`, `stop_timer`, `add_entry`, `update_entry`, `delete_entry`, `report`, `list_teams`, `list_users` (admin).

Tool descriptions state the permission each one needs, so an agent can explain a refusal instead of retrying. Tools that create an entry take its IANA `timezone`, defaulting to the machine's zone. No MCP resources; the API is the source of truth.

## Web UI

Svelte 5 + Vite, built to static assets and embedded with `go:embed`. The SPA calls the same API; no server-side rendering. `/` serves the public homepage. Signing in goes to `/login`, which redirects to `/w/{slug}/login` when there is one workspace and asks for the slug when there are several; every in-app link and API call keeps the prefix. The homepage offers **Create a workspace**, leading to `/signup` (email) and `/signup/verify?token=...` (the workspace form); when email is not configured the signup page says so instead of creating anything.

- **/timeline** — personal canvas timeline, infinite horizontally, wheel to zoom (day ↔ week), drag to create, drag edges to retime, drag body to move, click to edit project and note, running entry pinned at "now", overlaps marked, project colors, 15-minute snap with `Alt` for 1-minute precision, `space` start/stop, `n` new, `Esc` cancel, `Delete` remove.
- **/reports** — a summary strip (total hours, people who logged, average per person, busiest project and its share) over one table: projects as rows, one column per day, ISO week or calendar month across the range, plus a total column; rows ordered by hours, biggest first, with a grand total row at the bottom. Click a project to unfold one line per person. A Day | Week | Month switch, a range preset (this week / last week / this month / custom) that picks the granularity, a team filter when more than one team is readable, and CSV. Read-only; leads and admins only.
- **/admin/users**, **/admin/teams**, **/admin/projects** — tables with inline create and edit. No separate settings screen; a workspace is identified by its slug.

Leads do not get a team timeline in v1; the report table answers their question.

## Operations

Production is one VM: `tstamps-server` under systemd, Caddy terminating TLS on the same host, no container ([ADR-0009](./adr/0009-deploy-on-one-vm.md)). The unit files and Caddyfile live in `deploy/`. The server binds `127.0.0.1` and honors `X-Forwarded-Proto` for cookies and its last `X-Forwarded-For` hop for client addresses only from a loopback peer, so a direct client cannot forge either.

- **Backup** — `tstamps-server backup [--db path] <destination>` writes a consistent snapshot of the one database with SQLite's `VACUUM INTO`. The destination is a new file, written mode `0600`, published only when the snapshot succeeds. The snapshot is a single point in time for every workspace. It is safe while the server is running and refuses an existing destination, so a date-stamped name never overwrites yesterday. The deploy timer runs it nightly and keeps 14 days. Operators copy that file off the box; a backup on the same disk is not a backup.
- **Restore** — stop the server, replace the database file, remove stale `-wal`/`-shm`, start the server. `AutoMigrate` runs once at boot for every workspace. Restore is verified by `/healthz` and one report read. It restores every workspace together: there is no per-workspace restore or export command.
- **Schema version** — the applied version is stored in `PRAGMA user_version`; a workspace's `/w/{slug}/healthz` reports it, and the root `/healthz` only checks reachability. A binary refuses a database written by a newer schema; there is no downgrade path, restore the backup instead.
- **Retention** — logged hours are the company's timekeeping record. A leaver is deactivated, not deleted; their entries stay visible. Erasing a person means erasing the workspace. There is no per-user erasure in v1. Deleting a workspace is a transaction across its rows, not removing a file, and stays out of scope until a command exists.
- **Self-service signup** — off unless the server has `--email-api-key`, `--email-from` and `--public-url` (and optionally `--email-api-url`), which makes it an email dependency on Resend's API. Pending signups are rows in the same database, keyed by the token hash and holding only the address; they never expire, belong to no workspace, and are claimed by deleting the row when the form is submitted. Until then nothing is created, so an unverified address cannot make a workspace. An operator still cannot suspend or delete a workspace from the app.
- **Upgrade** — stop, back up, replace the binary, start. Schema changes are additive via `AutoMigrate`; a newer database is refused rather than guessed at.

## Layout and stack

```
cmd/tstamps/          user CLI: main, client subcommands, MCP
cmd/tstamps-server/   server: main, serve, workspace, backup, openapi
deploy/             systemd units, backup timer and Caddyfile
e2e/                the acceptance walk over both binaries
internal/config/    flags, env, config file
internal/store/     GORM models, AutoMigrate, queries
internal/workspace/ workspace slugs, creation, signup, shared registry and /w routing
internal/auth/      argon2id passwords, sessions, tokens, rate limit
internal/entry/     entry and timer rules
internal/report/    bucketing, aggregation, CSV
internal/api/       Huma operations, host-level signup handlers, middleware, permission checks
internal/client/    typed HTTP client shared by CLI and MCP
internal/cli/       subcommands
internal/mcp/       MCP tool definitions
web/                Svelte 5 + Vite source; web/dist is embedded
web/e2e/            Playwright smoke test against the built server
web/src/lib/api/    generated OpenAPI types (gitignored); the typed fetch wrapper sits beside them
```

Go stdlib `net/http` behind Huma's adapter. `log/slog` for logs. No DI framework: `main` wires a `store`, an `auth`, and an `api.Server` explicitly. The GORM models are the schema: `AutoMigrate` runs at boot, followed by one raw `CREATE UNIQUE INDEX ... WHERE end_at IS NULL` for the single running entry, which tags cannot express ([ADR-0005](./adr/0005-gorm-pure-go-sqlite.md)). The same boot path records `SchemaVersion` in `PRAGMA user_version` and refuses a database that reports a newer one.

One build pipeline, in this order: `go run ./cmd/tstamps-server openapi > web/openapi.json`, `openapi-typescript` into `web/src/lib/api/schema.d.ts`, `vite build` into `web/dist`, `go build` embeds it. `just dev` runs the API and Vite together from a Procfile; `just build` runs the pipeline. No generated types are committed.

## Testing

`go test ./...` must pass offline, with no external services.

- **entry/report unit tests** — local-midnight splits, DST spring-forward and fall-back, ISO week edges around New Year, calendar month edges, 14-hour padding, overlap detection, CSV escaping.
- **store tests** — `AutoMigrate` on an empty temp-dir database, a second boot being a no-op, the schema version being recorded and a newer one refused, the partial index actually rejecting a second running entry, archived-project filtering, deleted-entry exclusion, soft delete and restore, and a backup that can be opened with the rows intact.
- **API tests** — `httptest` with the full router; table-driven permission matrix (member / lead of that team / lead of another team / admin) over every endpoint; session and bearer auth incl. expired and revoked; range validation; 409 on double start; `/healthz` failing when the database is gone; security headers; the request body cap; the access-log line. One test asserts `/openapi.json` lists every route in the spec table, so a missing Huma registration fails CI.
- **CLI tests** — subcommands against a fake `client.Interface`. The `e2e` package builds both binaries, bootstraps a workspace with its first admin, serves on a free port, logs in, creates a project and entry, reads a report and a CSV, performs an MCP initialize + tool call over stdio, and runs signup against a stand-in email endpoint.
- **Workspace tests** — slug/path validation, atomic creation with its first admin, all-workspace backup, unknown slugs writing nothing, and two workspaces with the same email where neither a session nor an API token crosses workspaces. Signup is tested verify-first: no workspace exists before the link is used, a token works once, an invalid form leaves the link usable, a slug collision keeps it usable, a mail failure leaves nothing, and the pending row lives in the same database. The single-workspace `/login` redirect and the multi-workspace prompt are both covered. A path-bearing client base URL is tested against the router.
- **Frontend** — `pnpm run check` (svelte-check + tsc) runs against freshly generated types, so an API change the UI did not follow fails the build. Vitest covers timeline math (pixels ↔ time, snapping, resize clamps) and workspace-prefixed CSV URLs. Playwright runs a smoke walk on a one-workspace server and a prompt walk on a two-workspace server — choose a workspace, log in, reload a deep link, reject an unknown slug, and reach signup from the homepage — in CI.
- **CI** — `ci.yml` runs vet, `go test -race`, the web checks, both builds and the browser smoke test on every push and pull request; the release workflow reuses it, so a tag cannot ship code that did not pass.

Time is injected (`Clock` interface) anywhere "now" matters; tests never sleep.

## Build order

1. **Walking skeleton** — config, SQLite, AutoMigrate, Huma + `/openapi.json`, `users`, password login, session cookie, `/healthz`, `tstamps serve`, `tstamps workspace create`, first tests.
2. **Structure** — teams, memberships, projects, permission middleware, personal API tokens, `tstamps login`.
3. **Time** — entries, timer, overlap handling, report bucketing, CSV, permission matrix tests.
4. **Web** — login, timeline, reports, admin screens, typed client generation, embedded build.
5. **Agents** — CLI completion, MCP tools, end-to-end test, README with install and backup notes.
6. **Workspaces in one database** — every workspace as a row, `/w/{slug}` routing, operator creation and backup, single-workspace login redirect.
7. **Self-service signup** — verified email, workspace creation from the link, homepage entry point, email configuration and its off switch.

v1 is done when a fresh install can: create a workspace and its first admin from the CLI, add two teams and a lead each, create a member, log a day on the timeline, start and stop the timer from the CLI, export the week as CSV, and let an MCP client report last week's hours. Multi-workspace mode is done when an operator can create two slugged workspaces in one database, log into one by URL, and back both up in one snapshot without the API ever reaching the other. Signup is done when a visitor on a configured server can receive the link, create a workspace and land on its timeline signed in, and the same server with email unset creates nothing.
