# Hosted workspaces

One `tstamps-server` binary runs either one workspace or several. The difference is where the databases live and what the URLs look like:

| Shape | Storage | URLs |
| --- | --- | --- |
| One company | one SQLite file, `--db` | `/timeline`, `/reports`, `/admin` |
| Operator-hosted | one SQLite file per workspace, `--workspace-dir` | `/w/{slug}/timeline`, `/w/{slug}/reports`, `/w/{slug}/admin` |

Hosted mode is something an operator runs on their own VM with this binary. It is not a managed service: no operator control panel, no third party holding the workspace data, and no billing. You run the process and the backups. Self-service signup is optional and needs an email provider you configure; see below.

## When to use hosted mode

Use `--workspace-dir` when several companies share one server and must not see each other's data. Each workspace gets its own users, teams, projects, entries, tokens and database file, and its own admins.

For one company, stay on `--db`. It is one less directory to operate, the URLs stay unprefixed, and nothing about the workspace changes.

## Create a workspace

On the server host, as the user that runs the service:

```sh
sudo -u tstamps tstamps-server workspace create example.com \
  --workspace-dir /var/lib/tstamps/workspaces \
  --email you@example.com --name "Your Name"
```

It prints `created workspace example.com with admin you@example.com and initial password: ...`. Copy the password; like every initial password in Tstamps it is shown once, and the admin changes it at first sign-in.

The command writes the database and its first admin under a temporary name and links it into place when complete, so a half-created workspace is never served. Creating a workspace while the server runs is safe; it is picked up on the next request. An existing slug is refused.

Then serve the directory:

```sh
tstamps-server serve --addr :8080 --workspace-dir /var/lib/tstamps/workspaces
```

The directory must exist before `serve` starts; the first `workspace create` creates it. `--db` and `--workspace-dir` are mutually exclusive, and the server refuses to start with both.

`workspace create` makes only the first admin. That admin adds the company's other users, teams and projects in the [admin console](./admin.md), exactly as in one-workspace mode.

## Self-service signup

A hosted server can offer two ways in: an operator creates a workspace with `workspace create`, or a visitor creates one from the homepage. Signup is off unless you configure email, because the first step has to verify that the person owns the address.

```sh
tstamps-server serve --addr :8080 --workspace-dir /var/lib/tstamps/workspaces \
  --email-api-key re_... \
  --email-from "Tstamps <hello@example.com>" \
  --public-url https://time.example.com
```

- `--email-api-key` (`TSTAMPS_EMAIL_API_KEY`) is the Resend API key.
- `--email-from` (`TSTAMPS_EMAIL_FROM`) is the verified sending address. Verify the domain at Resend first, or the API rejects the send.
- `--public-url` (`TSTAMPS_PUBLIC_URL`) is the origin in the emailed link. It must match the address people use, including the scheme; a wrong value sends them to the wrong host.
- `--email-api-url` (`TSTAMPS_EMAIL_API_URL`) defaults to `https://api.resend.com/emails`. Change it only for a Resend-compatible relay or a test double.

All three of key, from and public URL are required together; the server refuses a partial configuration. With none set, the homepage still shows **Create a workspace**, but the page answers that signup is not enabled and creates nothing. That is also the off switch: remove the key and restart.

The flow is two pages:

1. `/signup` asks for an email. The server stores a pending record and sends a single-use link. Sending is limited to five per hour per IP.
2. The link opens `/signup/verify?token=...`, which shows the verified address and asks for the workspace URL, the person's name and a password (at least 8 characters). Submitting creates the workspace and its first admin, then signs that admin in and opens `/w/{slug}/timeline`.

Nothing is written to the workspace directory until the second page is submitted, so an unverified address cannot create files. The pending records are `0600` files under `<workspace-dir>/pending/`, named by the token hash, not by slug; they are not workspaces, are not in a backup, and never expire. Delete that directory while the server is stopped to cancel every outstanding link. A used link is dead; asking again from `/signup` sends a new one.

The created admin is an ordinary workspace admin: they can add users, teams and projects, and there is still no server-wide role. The operator remains the only one who can suspend or delete a workspace, and that is still a filesystem action (see [Deliberately out of scope](#deliberately-out-of-scope)).

Signup leaves no public list of workspaces. `/login` asks for a slug and reveals only whether the one you typed exists, exactly as before.

## The slug

The slug is the workspace's name in the URL and the name of its database file: `example.com` is stored as `example.com.db` and served at `/w/example.com/`.

It is chosen by the operator, or by the person signing up, and is not a DNS name — no subdomain or certificate is involved. Use a short, stable, lowercase label that tells you which company it is. The rules:

- 1 to 63 characters
- lowercase letters, digits, dots and hyphens
- no leading or trailing dot or hyphen, no `..`

There is no rename command in v1: changing the slug means a new file and a new URL, so pick it before you hand the URL to the company.

## URLs and login

Open `/` for the homepage, then sign in to get the workspace prompt at `/login`; it takes you to `/w/{slug}/login`. From there every page and API route keeps the prefix:

| Page | One workspace | Hosted |
| --- | --- | --- |
| Timeline | `/timeline` | `/w/{slug}/timeline` |
| Reports | `/reports` | `/w/{slug}/reports` |
| Admin console | `/admin` | `/w/{slug}/admin` |
| API | `/api/v1/...` | `/w/{slug}/api/v1/...` |
| API reference | `/api/v1/docs` | `/w/{slug}/api/v1/docs` |
| Health | `/healthz` | `/w/{slug}/healthz` |

An unknown or malformed slug answers `404`; it never creates a workspace. Unprefixed `/api/` routes answer `404` in hosted mode, because there is no workspace to guess.

A browser holds one workspace login at a time per server. Sessions share one cookie, so logging into another workspace replaces it and the earlier tab is signed out. Use a second browser profile or a private window when you must be in two workspaces at once.

## Isolation

Each workspace is a separate SQLite file. The workspace is chosen from the URL before authentication, and a session or API token is looked up only in that file.

- The same email in two workspaces is two accounts with separate passwords, profiles and tokens.
- A token minted in one workspace is rejected by every other workspace.
- Admins are workspace-local. There is no server-wide admin role and no cross-workspace report.

One process serves all workspaces, so SQLite's single-writer limit applies per file: one workspace's writes do not block another's.

## Point the CLI and MCP at a workspace

The workspace lives in the base URL, not in a flag:

```sh
tstamps login --url https://time.example.com/w/example.com
```

The config file keeps that URL, so every later command stays in the workspace. Run `tstamps login` again with another URL to switch. For MCP, the same form goes in `TSTAMPS_URL`; see [MCP](./mcp.md).

## Health

- `GET /healthz` answers `200` while the workspace directory is readable and `503` when it is not. It does not touch any workspace database.
- `GET /w/{slug}/healthz` answers `200` while that workspace's SQLite file is reachable and reports the schema version stored in it; `503` otherwise.

Monitor the root for the process and each workspace you care about. An unknown slug also answers `404`, so a typo in a monitor looks different from an outage.

## Backup and restore

Take one snapshot per workspace into a new directory:

```sh
sudo -u tstamps tstamps-server backup \
  --workspace-dir /var/lib/tstamps/workspaces \
  "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ)"
```

- The destination must not exist and must be outside the workspace directory.
- The result is one `<slug>.db` per workspace, published only when every snapshot succeeds. A failed run leaves nothing behind to mistake for a backup.
- Each file is internally consistent; the set is not one atomic cross-workspace instant.
- It is safe while the server is running, and it refuses to run when the directory holds no workspaces.

Restore one company without touching the others: stop the server, replace `<slug>.db` in the workspace directory, remove that file's stale `-wal` and `-shm`, start the server. Restoring every workspace is the same with the whole directory. Verify with `curl -fsS https://time.example.com/w/{slug}/healthz` and by reading one report.

The shipped [backup timer](https://github.com/havspect/tstamps/tree/main/deploy) is written for one database. For hosted mode, replace its `--db` with `--workspace-dir` and make its cleanup delete old snapshot directories rather than old `.db` files.

## Upgrade

The upgrade is the same as for one workspace: stop the server, take a backup, install the new `tstamps-server`, start it.

Migrations run at boot in one-workspace mode, and on first open of each hosted workspace after the upgrade. A workspace stays unopened — and unmigrated — until its first request. A binary refuses a file written by a newer schema, so there is no silent downgrade; restore the backup to go back. See [Backup and upgrade](./backup-upgrade.md).

## Deliberately out of scope

- **A workspace list.** `/` asks for a slug and does not reveal which ones exist, so one company cannot enumerate the others.
- **Billing, plans and quotas.** Nothing meters seats or usage.
- **Signup without email.** Verification needs a sending domain and an API key; there is no invite-code or open form that skips it.
- **Suspension and deletion workflows.** There is no UI, API or CLI command for either. To stop serving a workspace, stop the server, then move or delete `<slug>.db` (and its `-wal`/`-shm`). Do it stopped: an open handle would otherwise keep serving a file you meant to remove. Backups are deleted the same way, by hand.
- **Server-wide administration, SSO and cross-workspace reporting.** The **Admin** role always means one workspace, and each company's numbers stay in its own reports.

## Next

- [Run the server](./server.md) on a Linux host.
- [Back up and upgrade](./backup-upgrade.md) without losing hours.
- [Point the CLI](./install-cli.md) at one workspace.
