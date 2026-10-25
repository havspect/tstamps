# Tstamps

**Team time tracking: log time by dragging on a timeline; whoever needs the
numbers reads a report.** One server binary holding the web timeline, the REST
API and one SQLite database per workspace, plus a CLI that every user installs
on their own machine and points at that server. Run it for one company, or host
several companies as separate workspaces. No per-seat subscription.

The manual lives in [docs/guide](docs/guide) and ships inside the server at
`/docs`. This README is the front door.

## Why Tstamps

- **Logging is a timeline, not a timesheet.** Drag on a lane to create an
  entry, drag its edges to retime it, press `space` to start and stop a running
  entry.
- **Your hours land in the day they happened.** Every entry records the
  timezone it was logged in, so a day worked from Sydney counts as Sydney's
  Monday ([ADR-0007](docs/adr/0007-entry-timezones-decide-buckets.md)).
- **Leads read the hours without asking for a form.** Anyone may log on any
  project, and a lead reads every hour booked on their team's projects
  ([ADR-0002](docs/adr/0002-teams-own-projects.md)).
- **Nothing disappears by accident.** Deleting hides, archiving keeps the
  hours, a leaver is deactivated; an admin restores any of it without a backup
  ([ADR-0004](docs/adr/0004-archive-projects-delete-entries.md)).
- **Yours to run.** One `tstamps-server` process on one VM, behind Caddy on the
  same host, and a backup that is a file copy
  ([ADR-0009](docs/adr/0009-deploy-on-one-vm.md)). Hosted deployments keep one
  SQLite file per workspace, so backup and isolation stay per company
  ([ADR-0011](docs/adr/0011-host-workspaces-as-separate-sqlite-files.md)).

Not for you if you need approvals, leave, invoicing or rates; a managed
service, SSO or an audit log; per-user erasure; several companies inside one
workspace; or realtime co-editing. The full scope is [docs/SPEC.md](docs/SPEC.md).

## Install the CLI

Take the archive for your machine from the
[latest release](https://github.com/havspect/tstamps/releases/latest). Linux
amd64 shown; `arm64`, macOS and the Windows zip work the same way:

```sh
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
```

Then log in against your workspace:

```sh
tstamps login --url https://time.example.com
tstamps whoami
```

On an operator-hosted server the URL names the workspace, for example
`https://time.example.com/w/example.com`. All three platforms, checksums and
config paths: [Install the CLI](docs/guide/install-cli.md).

## Run a workspace

The server is built where it runs and targets Linux; it needs Go 1.26+, [pnpm](https://pnpm.io)
and [just](https://github.com/casey/just).

```sh
pnpm install
just build-prod
tstamps-server admin create --email you@example.com --name "Your Name" --db ./tstamps.db
tstamps-server serve --addr :8080 --db ./tstamps.db
```

Open `http://localhost:8080`, change the initial password, then create teams,
users and memberships under **/admin**. Deployment with systemd and Caddy is in
[Server on Linux](docs/guide/server.md).

### Host several companies

`--workspace-dir` keeps one SQLite file per workspace and selects it by slug in
the URL:

```sh
tstamps-server workspace create example.com \
  --workspace-dir ./workspaces --email you@example.com --name "Your Name"
tstamps-server serve --addr :8080 --workspace-dir ./workspaces
```

`--db` and `--workspace-dir` are mutually exclusive. Each workspace has its own
users and data at `/w/{slug}/`; the same email in two workspaces is two
accounts. Operators create, suspend and delete workspaces. A hosted server may
also offer self-service signup when it is given an email API key, a From address
and a public URL; without those it stays operator-only. The full shape is in
[Hosted workspaces](docs/guide/workspaces.md).

## The manual

| Page | What it covers |
| --- | --- |
| [Overview](docs/guide/index.md) | The pieces, the roles and a quick start |
| [Install the CLI](docs/guide/install-cli.md) | Linux, macOS, Windows, checksums, config paths |
| [Timeline](docs/guide/timeline.md) | Dragging, keyboard, running entries, overlaps |
| [Reports](docs/guide/reports.md) | Reading the table and the CSV |
| [CLI reference](docs/guide/cli.md) | Every command on every platform |
| [MCP](docs/guide/mcp.md) | Agent setup, tools and permissions |
| [Admin console](docs/guide/admin.md) | Users, teams, projects, restore |
| [Server on Linux](docs/guide/server.md) | Build, systemd, Caddy, settings |
| [Hosted workspaces](docs/guide/workspaces.md) | One workspace per company, slugs, backup |
| [Backup and upgrade](docs/guide/backup-upgrade.md) | Snapshots, restore, schema versions |

Vocabulary is [GLOSSARY.md](GLOSSARY.md); decisions worth not reversing are in
[docs/adr](docs/adr), including
[why the server and the CLI are two binaries](docs/adr/0008-user-cli-and-server-are-two-binaries.md).

## Development

```sh
pnpm install     # web and docs workspaces
just check       # go vet, go test, svelte-check, vitest, docs build
just dev         # API, Vite and the manual, all hot reloading
just e2e         # build the server, then the legacy and hosted Playwright walks
```

`just docs-dev` previews the manual at `http://localhost:5174/docs/`.
`just e2e` needs the browser once:
`pnpm --dir web exec playwright install chromium`.

`go test ./...` passes offline with no external services. Time is injected
where "now" matters; tests never sleep. CI runs the same gate and the browser
smoke test on every push and pull request.

## License

AGPL-3.0-only; see [LICENSE](LICENSE).
