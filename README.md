# Tstamps

**Team time tracking for one company: log time by dragging on a timeline;
whoever needs the numbers reads a report.** One server binary holding the
SQLite database, the web timeline and the REST API, plus a CLI that every user
installs on their own machine and points at that server. No per-seat
subscription.

## Why Tstamps

- **Logging is a timeline, not a timesheet.** Drag on an infinite canvas to
  create an entry, drag its edges to retime it, press `space` to start and stop
  a running entry. A week of hours is a few drags, not a Friday form.
- **Your hours land in the day they happened.** Every entry records the
  timezone it was logged in, so a day worked from Sydney counts as Sydney's
  Monday and a lead in Berlin reads the same number
  ([ADR-0007](docs/adr/0007-entry-timezones-decide-buckets.md)).
- **Leads read the hours without asking for a form.** Anyone may log on any
  project, and a lead reads every hour booked on their team's projects, by
  person, by day, week or month, with CSV
  ([ADR-0002](docs/adr/0002-teams-own-projects.md)).
- **Log from where you work.** `tstamps start` is one line in a terminal, and
  `tstamps mcp` lets an agent start, stop and correct entries with the
  permissions of the token you gave it — no browser tab.
- **Nothing disappears by accident.** Deleting an entry hides it, archiving a
  project keeps its hours, a leaver is deactivated with their history intact;
  an admin restores any of it without a backup
  ([ADR-0004](docs/adr/0004-archive-projects-delete-entries.md)).
- **Yours to run.** One `tstamps-server` process on one VM with the UI
  embedded, behind Caddy on the same host, and a backup that is a file copy
  ([ADR-0009](docs/adr/0009-deploy-on-one-vm.md)).

## Not for you if

- you need approvals, leave and holidays, invoicing or rates;
- you want a hosted service, SSO, an audit log or a mobile app;
- you must erase one person's data — a leaver is deactivated, and erasing a
  person means erasing the workspace;
- you need several companies on one install, or realtime co-editing.

What it does and does not do is [docs/SPEC.md](docs/SPEC.md). The vocabulary is
[GLOSSARY.md](GLOSSARY.md); decisions worth not reversing are in
[docs/adr](docs/adr), including
[why the server and the CLI are two binaries](docs/adr/0008-user-cli-and-server-are-two-binaries.md).

## Install the CLI

`tstamps` needs no database and no Go toolchain. Take the archive for your
machine from the [latest release](https://github.com/havspect/tstamps/releases/latest)
and put the binary on your `PATH`:

```sh
# Linux amd64 shown; swap amd64 for arm64, or tar for the windows zip.
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
```

Assets are `tstamps_{linux,darwin}_{amd64,arm64}.tar.gz`,
`tstamps_windows_amd64.zip` and a `checksums.txt`. Then log in against your
server, as shown under [CLI](#cli).

## Run a workspace

The server is `tstamps-server`; it embeds the web UI, so it is built where it
runs. Requirements: Go 1.26+, [pnpm](https://pnpm.io), [just](https://github.com/casey/just).

```sh
just build          # generate API types, build the SPA, embed it, write ./tstamps-server
sudo install -m 0755 tstamps-server /usr/local/bin/tstamps-server
```

`just build` regenerates the OpenAPI types, runs `vite build`, then compiles with
`-tags web` so the UI is embedded. `just build-prod` does the same with
`CGO_ENABLED=0`, `-trimpath` and stripped symbols, which is what you deploy. A
plain `go build ./cmd/tstamps-server` works for API work but serves a "UI is not
built" page. `just build-cli` builds `./tstamps` for local testing of the CLI.

## First run

```sh
tstamps-server admin create --email you@example.com --name "Your Name" --db ./tstamps.db
tstamps-server serve --addr :8080 --db ./tstamps.db
```

Open `http://localhost:8080` and log in with the initial password the first
command printed. Change it when prompted, then create teams, users and
memberships under **/admin**. The CLI reads those; it does not create them.

## Deploy on a VM

Production is one VM running `tstamps-server` under systemd, with Caddy terminating
TLS on the same host ([ADR-0009](docs/adr/0009-deploy-on-one-vm.md)). The unit
files and Caddyfile are in [`deploy/`](deploy).

```sh
# On the VM, after `just build-prod` and copying tstamps-server over:
sudo useradd --system --home /var/lib/tstamps --shell /usr/sbin/nologin tstamps
sudo install -m 0755 tstamps-server /usr/local/bin/tstamps-server
sudo install -d -o tstamps -g tstamps -m 0700 /var/backups/tstamps
sudo install -m 0644 deploy/tstamps-server.service deploy/tstamps-backup.service \
  deploy/tstamps-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now tstamps-server tstamps-backup.timer

# Edit the hostname in deploy/Caddyfile first.
sudo install -m 0644 deploy/Caddyfile /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Create the first admin against the live database (the CLI reads and writes the
same file; the server keeps serving):

```sh
sudo -u tstamps tstamps-server admin create \
  --db /var/lib/tstamps/tstamps.db --email you@example.com --name "Your Name"
```

The unit binds `127.0.0.1:8080`; Caddy is the only public listener and sets the
`X-Forwarded-Proto` header the server needs to mark cookies `Secure`, plus the
`X-Forwarded-For` hop it uses for client IPs in logs and login rate limiting. The
server trusts both only from a loopback peer, so a direct client that sends them
is ignored; keep port 8080 off the network anyway. `GET /healthz` answers 200
only while SQLite is reachable, and reports the schema version stored in the
file.

## CLI

The CLI talks to the API, never to the database. Log in once per machine; the
token is written to `~/.config/tstamps/config.json` with mode `0600`.

```sh
tstamps login --url http://localhost:8080

tstamps whoami
tstamps projects add Timeline --team Platform
tstamps add Timeline --date today --from 09:00 --to 11:30 --note "drag, resize"
tstamps start Timeline --note "OAuth callback"
tstamps stop
tstamps ls
tstamps report --from 2026-10-19 --to 2026-10-26 --by week
tstamps report --from 2026-10-19 --to 2026-10-26 --csv > week.csv
```

A user an admin created or reset must change the initial password in the web UI
before `tstamps login` can mint a token.

Times without a date mean today in your timezone; `--to 1h30m` and
`--to 01:00` (crossing midnight) both work. `tstamps help` lists every command.
Exit codes: `0` ok, `1` error, `2` usage.

Server settings come from flags, then the environment, then defaults:
`--addr`/`TSTAMPS_ADDR` (default `:8080`) and `--db`/`TSTAMPS_DB` (default
`./tstamps.db`). Client settings come from `--url`/`--token`, then `TSTAMPS_URL`/
`TSTAMPS_TOKEN`, then the config file.

## MCP

`tstamps mcp` speaks MCP over stdio and uses the same config and token as the CLI.
Read tools are annotated read-only; each description states the permission it
needs, so an agent can explain a refusal.

```json
{
  "mcpServers": {
    "tstamps": {
      "command": "tstamps",
      "args": ["mcp"],
      "env": {
        "TSTAMPS_URL": "https://tstamps.example.com",
        "TSTAMPS_TOKEN": "ts_..."
      }
    }
  }
}
```

Tools: `whoami`, `list_projects`, `list_entries`, `start_timer`, `stop_timer`,
`add_entry`, `update_entry`, `delete_entry`, `report`, `list_teams` (admin),
`list_users` (admin). Mint a token for an agent with `tstamps tokens create agent`;
it is shown once.

## Backup

Everything lives in one SQLite file (`tstamps.db` by default) plus its `-wal` and
`-shm` companions while the server runs.

- **With the server running:** `tstamps-server backup` takes a consistent
  snapshot with SQLite's `VACUUM INTO`. It refuses an existing file, so a
  date-stamped name never overwrites yesterday:

  ```sh
  tstamps-server backup --db /var/lib/tstamps/tstamps.db \
    "/var/backups/tstamps/tstamps-$(date -u +%Y%m%dT%H%M%SZ).db"
  ```

  `deploy/tstamps-backup.timer` runs that nightly and deletes files older than
  14 days.

- **With the server stopped:** copy `tstamps.db` (and the WAL/SHM files if they
  are still there), or copy the whole directory.

- **Off the box:** a snapshot on the same disk is not a backup. Copy
  `/var/backups/tstamps` with restic, borg, rclone or whatever the host already
  uses.

- **Restore:** stop the server, replace `tstamps.db` (remove stale `-wal`/`-shm`),
  start it, check `curl -fsS http://127.0.0.1:8080/healthz`, and read one report.
  Migrations run at boot.

- The client config holds a bearer token and is a credential, not data worth
  backing up.

Nothing is hard-deleted: deleted entries keep their row with `deleted_at`, and
archived projects keep their hours, so an admin can restore either without a
backup.

## Upgrade

Stop the server, take a backup, install the new `tstamps-server`, start it.
`AutoMigrate` runs at boot and records the schema version in the database file;
`GET /healthz` reports that version. A binary refuses to start on a database
written by a newer schema, so there is no silent downgrade — restore the backup
to go back.

## Development

```sh
just check       # go vet, go test, svelte-check, vitest
just e2e         # build the server, then one Playwright smoke walk
just dev         # API plus Vite dev server, both hot reloading
just serve-watch # API only, hot reloaded (UI comes from Vite)
```

`just e2e` needs the browser once: `pnpm --dir web exec playwright install chromium`.
CI runs the same smoke test against the built server.

`just dev` needs [hivemind](https://github.com/DarthSim/hivemind) and
[air](https://github.com/air-verse/air) (`go install
github.com/air-verse/air@latest`). Air rebuilds and restarts the API on Go
changes; Vite hot-reloads the UI on the same page.

`go test ./...` passes offline with no external services. The end-to-end test
builds both binaries, serves on a free port, bootstraps an admin, logs an entry,
exports CSV and calls the MCP server over stdio. The Playwright smoke test drives
the built UI: login, an entry on the timeline, the report table and the CSV.

## License

AGPL-3.0-only; see [LICENSE](LICENSE).
