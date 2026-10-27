# Workspaces

One `tstamps-server` process serves one SQLite database, and that database holds every workspace. A server can hold one company's workspace or several; each is addressed at `/w/{slug}/...`, and the slug is selected from the URL before authentication.

| Thing | Where |
| --- | --- |
| Storage | one SQLite database, `--db` (`./tstamps.db` by default, `TSTAMPS_DB`) |
| Workspace | a row in `workspaces`; every tenant row carries `workspace_id` |
| URLs | `/w/{slug}/timeline`, `/w/{slug}/reports`, `/w/{slug}/admin` |
| Operator CLI | `tstamps-server workspace create <slug> --email x --name y` (always available) |
| Self-service signup | `/signup` with email verification (off unless `--email-api-key`, `--email-from` and `--public-url` are set) |

There is no separate storage mode: a self-hosted install for one company and a server running several companies use the same binary, the same schema and the same `/w/{slug}` URLs. By default, workspaces are created from the CLI with `tstamps-server workspace create`; self-service signup over the web is disabled until email verification is configured.

## Create a workspace

On the server host, as the user that runs the service:

```sh
sudo -u tstamps tstamps-server workspace create example.com \
  --db /var/lib/tstamps/tstamps.db \
  --email you@example.com --name "Your Name"
```

It creates the database if it does not exist, migrates it, inserts the workspace and its first admin in one transaction, and prints `created workspace example.com with admin you@example.com and initial password: ...`. Copy the password; like every initial password in Tstamps it is shown once, and the admin changes it at first sign-in.

The same command on an existing database adds another workspace as a row. An existing slug is refused. Creating a workspace while the server runs is safe; it is picked up on the next request.

Then serve the database:

```sh
tstamps-server serve --addr :8080 --db /var/lib/tstamps/tstamps.db
```

Open `http://localhost:8080/w/example.com/`. The root `/` is the public homepage; `/login` takes you to the workspace's sign-in page (see below).

`workspace create` makes only the first admin. That admin adds the company's other users, teams and projects in the [admin console](./admin.md).

## Sign in

Every workspace keeps its own users, so the URL has to name the workspace before a password is checked.

- A server with exactly one workspace skips the prompt: `GET /login` answers `302` and redirects to `/w/{slug}/login`. The URL still carries the slug; the user is simply not asked to type it.
- A server with two or more workspaces shows the workspace prompt at `/login`. Entering a slug leads to `/w/{slug}/login`; an unknown slug stays on the prompt with an error.

A browser holds one workspace login at a time per server. Sessions share one cookie, so logging into another workspace replaces it and the earlier tab is signed out. Use a second browser profile or a private window when you must be in two workspaces at once.

## Self-service signup

A server can offer two ways in: an operator creates a workspace with `workspace create`, or a visitor creates one from the homepage. Signup is off unless you configure email, because the first step has to verify that the person owns the address.

```sh
tstamps-server serve --addr :8080 --db /var/lib/tstamps/tstamps.db \
  --email-api-key re_... \
  --email-from "Tstamps <hello@example.com>" \
  --public-url https://time.example.com
```

- `--email-api-key` (`TSTAMPS_EMAIL_API_KEY`) is the Resend API key.
- `--email-from` (`TSTAMPS_EMAIL_FROM`) is the verified sending address. Verify the domain at Resend first, or the API rejects the send.
- `--public-url` (`TSTAMPS_PUBLIC_URL`) is the origin in the emailed link. It must match the address people use, including the scheme; a wrong value sends them to the wrong host.
- `--email-api-url` (`TSTAMPS_EMAIL_API_URL`) defaults to `https://api.resend.com/emails`. Change it only for a Resend-compatible relay or a test double.

All three of key, from and public URL are required together; the server refuses a partial configuration. With none set, the homepage still shows **Create a workspace**, but submitting `/signup` (or opening `/signup/verify`) answers `503` that signup is not enabled and creates nothing. That is also the off switch: remove the key and restart.

The flow is two pages:

1. `/signup` asks for an email. The server stores a pending record and sends a single-use link. Sending is limited to five per hour per IP.
2. The link opens `/signup/verify?token=...`, which shows the verified address and asks for the workspace URL, the person's name and a password (at least 8 characters). Submitting creates the workspace row and its first admin, then signs that admin in and opens `/w/{slug}/timeline`.

Pending signups are rows in the same database, keyed by the token hash and holding only the email address. Nothing is created until the second page is submitted, so an unverified address cannot create a workspace. Submitting claims the row by deleting it, so one link creates one workspace even if two requests arrive together; a form that fails validation keeps the link usable. Pending rows never expire, and they are in the database, so they are part of a backup. An operator who wants to cancel every outstanding link deletes the `pending_signups` rows.

The created admin is an ordinary workspace admin: they can add users, teams and projects, and there is still no server-wide role. The operator remains the only one who can suspend or delete a workspace, and that is still out of the app (see [Deliberately out of scope](#deliberately-out-of-scope)).

Signup leaves no public list of workspaces. The prompt at `/login` reveals only whether a slug you typed exists, and with one workspace it does not ask at all.

## The slug

The slug is the workspace's name in the URL: `example.com` is served at `/w/example.com/`. It is a row in the database, not a file name.

It is chosen by the operator, or by the person signing up, and is not a DNS name — no subdomain or certificate is involved. Use a short, stable, lowercase label that tells you which company it is. The rules:

- 1 to 63 characters
- lowercase letters, digits, dots and hyphens
- no leading or trailing dot or hyphen, no `..`

There is no rename command in v1: changing the slug means a new URL, so pick it before you hand the URL to the company.

## URLs and login

Open `/` for the homepage, then sign in to reach `/w/{slug}/login`. Every page and API route keeps the prefix:

| Page | URL |
| --- | --- |
| Timeline | `/w/{slug}/timeline` |
| Reports | `/w/{slug}/reports` |
| Admin console | `/w/{slug}/admin` |
| API | `/w/{slug}/api/v1/...` |
| API reference | `/w/{slug}/api/v1/docs` |
| Workspace health | `/w/{slug}/healthz` |

An unknown or malformed slug answers `404`; it never creates a workspace. Unprefixed `/api/` routes answer `404`, because there is no workspace to guess. Only the public homepage, the sign-in prompt, the signup pages, the manual at `/docs` and the root health check answer without a slug.

## Isolation

Every workspace's rows live in one SQLite database, scoped by `workspace_id`. The workspace is chosen from the URL before authentication, and a session or API token is looked up only within that workspace.

- The same email in two workspaces is two accounts with separate passwords, profiles and tokens.
- A token minted in one workspace is rejected by every other workspace.
- Admins are workspace-local. There is no server-wide admin role and no cross-workspace report.
- An ID from one workspace is `404` in another, on reads and on writes.

One process serves all workspaces, so SQLite's single-writer rule applies to the whole server: a busy workspace can make another wait up to the 10-second busy timeout. That is the capacity trade of one database, and the reason to keep the server on one VM.

## Point the CLI and MCP at a workspace

The workspace lives in the base URL, not in a flag:

```sh
tstamps login --url https://time.example.com/w/example.com
```

The config file keeps that URL, so every later command stays in the workspace. Run `tstamps login` again with another URL to switch. For MCP, the same form goes in `TSTAMPS_URL`; see [MCP](./mcp.md).

## Health

- `GET /healthz` answers `200` while the database is reachable and `503` when it is not. It pings the database without opening a particular workspace.
- `GET /w/{slug}/healthz` answers `200` while that workspace exists and reports the schema version stored in the database; `404` for an unknown slug and `503` when the database cannot answer.

Monitor the root for the process and each workspace you care about. An unknown slug also answers `404`, so a typo in a monitor looks different from an outage.

## Backup and restore

Take one snapshot of the database into a new file:

```sh
sudo -u tstamps tstamps-server backup \
  --db /var/lib/tstamps/tstamps.db \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ).db"
```

- The destination must not exist. It is written as one file with mode `0600`.
- The result is one file that is a single point in time for every workspace, published only when the snapshot succeeds and written mode `0600`. A failed run leaves nothing behind to mistake for a backup.
- It is safe while the server is running, and it refuses to run when the database holds no workspaces.

Restore the whole server: stop it, replace the database file with the snapshot, remove stale `-wal` and `-shm` files next to it, start the server. Verify with `curl -fsS https://time.example.com/w/{slug}/healthz` and by reading one report.

Restoring one workspace from a whole-database snapshot is not a command yet. Until it is, the snapshot is all-or-nothing; keep that in mind before using one server for companies whose retention needs differ.

The shipped [backup timer](https://github.com/havspect/tstamps/tree/main/deploy) runs the same `--db` command nightly; nothing changes with more workspaces.

## Upgrade

The upgrade is the same for one workspace or many: stop the server, take a backup, install the new `tstamps-server`, start it.

Migrations run once at boot for every workspace in the database. A binary refuses a database written by a newer schema, so there is no silent downgrade; restore the backup to go back. There is no migration path from the earlier per-file layout or from a schema-5 `--db` database: nothing was deployed under either, so recreate them rather than upgrading in place.

## Deliberately out of scope

- **A workspace list.** The sign-in prompt asks for a slug and does not reveal which ones exist, so one company cannot enumerate the others.
- **Billing, plans and quotas.** Nothing meters seats or usage.
- **Signup without email.** Verification needs a sending domain and an API key; there is no invite-code or open form that skips it.
- **Suspension and deletion workflows.** There is no UI, API or CLI command for either, and deleting a workspace is a transaction across its rows, not removing a file. Until a command exists, treat both as out of scope rather than doing them by hand in SQL.
- **Server-wide administration, SSO and cross-workspace reporting.** The **Admin** role always means one workspace, and each company's numbers stay in its own reports.

## Next

- [Run the server](./server.md) on a Linux host.
- [Back up and upgrade](./backup-upgrade.md) without losing hours.
- [Point the CLI](./install-cli.md) at a workspace.
