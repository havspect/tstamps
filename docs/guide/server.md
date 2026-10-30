# Server on Linux

Tstamps server runs on Linux. The CLI runs on Linux, macOS and Windows; the server does not.

One `tstamps-server` process serves the web UI, the REST API and its SQLite storage: one database file holding every workspace at `/w/{slug}/...`. The UI is embedded in the binary, so there is no server release archive: build it from this checkout, on the host or on your machine for the host's architecture.

## Requirements

- Go 1.26 or newer
- [pnpm](https://pnpm.io)
- [just](https://github.com/casey/just)
- optionally, [Caddy](https://caddyserver.com) for TLS

## Build

```sh
git clone https://github.com/havspect/tstamps
cd tstamps
just build-prod
```

`just build-prod` regenerates the OpenAPI types, builds the SPA with Vite, embeds it with `-tags web`, and writes a static, stripped `./tstamps-server` with `CGO_ENABLED=0`. The SQLite driver is pure Go, so no C compiler is needed.

It builds for the machine it runs on. `just build-prod linux arm64` cross-compiles for the host, which is what `just deploy` uses.

For API work, `just build` leaves symbols in and is faster. A plain `go build ./cmd/tstamps-server` also works but serves a "UI is not built" page instead of the app.

## Run it locally first

```sh
tstamps-server workspace create example.com --db ./tstamps.db \
  --email you@example.com --name "Your Name"
tstamps-server serve --addr :8080 --db ./tstamps.db
```

The first command creates the database if needed and prints the workspace admin's initial password. Open `http://localhost:8080/w/example.com/`, sign in, change the password when prompted, then create teams, users and memberships under **/admin**. The CLI reads those; it does not create them.

`workspace create` opens the database directly. It works while the server is running, which is also how you add a workspace to a deployed instance. Running it again with another slug adds another workspace to the same database.

## Serve several companies

There is nothing else to enable. Each workspace is a row in the same database and is selected by the slug in the URL:

```sh
tstamps-server workspace create other.example --db ./tstamps.db \
  --email you@example.com --name "Your Name"
```

With one workspace in the database, `/login` redirects straight to that workspace's sign-in page. With two or more, `/login` asks for the slug first. The full shape is on [Workspaces](./workspaces.md).

To let visitors create their own workspace, add `--email-api-key`, `--email-from` and `--public-url`. Without them, signup is off. Details and the email setup are on [Workspaces](./workspaces.md#self-service-signup).

## Deploy on a VM

The production shape is one VM running `tstamps-server` under systemd, with Caddy terminating TLS on the same host. The unit files and Caddyfile are in [`deploy/`](https://github.com/havspect/tstamps/tree/main/deploy). The server binds loopback; Caddy is the only public listener.

On a fresh Debian or Ubuntu VM with SSH as root, one command does the whole first install: it builds the binary, installs Caddy, the service user, the units and the Caddyfile, starts both services and creates the first workspace.

```sh
just provision time.example.com you@example.com
```

The second argument is the workspace slug and Caddy hostname and defaults to the host; the third is the first admin's email, and without it no workspace is created. Point the domain's DNS at the VM first, or Caddy cannot get a certificate. The Caddyfile it writes is the production block only, without the demo. The same steps by hand, after `just build-prod` and copying `tstamps-server` to the VM:

```sh
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

Create the first workspace against the live database:

```sh
sudo -u tstamps tstamps-server workspace create example.com \
  --db /var/lib/tstamps/tstamps.db --email you@example.com --name "Your Name"
```

There is no server-wide admin: each workspace's first admin comes from its own `workspace create`. Add further workspaces the same way, with a different slug and email.

Caddy sets `X-Forwarded-Proto`, which is how the server knows to mark the session cookie `Secure`. It also sets `X-Forwarded-For`, the hop used for client IPs in logs and for login rate limiting. The server trusts both only from a loopback peer, so a direct client cannot forge them. Keep port 8080 off the network anyway.

## Run a public demo

A demo server seeds one fictional workspace and rebuilds it at the top of every
hour, so visitors may change anything without keeping it.

```sh
just provision-demo demo.example.com
```

That builds the binary and runs the same playbook as `just provision` with the
demo site only, on a host whose DNS already points at the VM. Run it against a
VM that already serves production and it adds the demo host beside the existing
site instead of replacing it; nothing is backed up either way.

After the first install, updates use `just deploy-demo demo.example.com [user]
[arch]`. It swaps the binary, restarts only `tstamps-demo`, and polls
`http://127.0.0.1:8081/healthz`; it leaves the database alone because the demo
reseeds hourly. The same steps by hand:

```sh
sudo install -d -o tstamps -g tstamps -m 0700 /var/lib/tstamps-demo
sudo install -d -o tstamps -g tstamps -m 0750 /var/lib/tstamps-demo/legal
sudo install -m 0644 imprint.html privacy.html /var/lib/tstamps-demo/legal/
sudo install -m 0644 deploy/tstamps-demo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now tstamps-demo
```

Its unit runs `tstamps-server serve --demo --addr 127.0.0.1:8081 --db
/var/lib/tstamps-demo/demo.db --legal-dir /var/lib/tstamps-demo/legal`; add the
`demo.` host from the Caddyfile and point it at port 8081. Nothing in that
database is worth backing up. The demo's Impressum and privacy notice are the
operator's own pages, kept out of the repository; supply your own pair.

At boot the server creates a workspace with the slug `demo` and seeds three
accounts, all with the password `demo`:

| Email | Role |
| --- | --- |
| `ada@example.com` | admin |
| `leo@example.com` | lead of Product |
| `mia@example.com` | member of Product |

The homepage offers **Try the live demo**, the login page lists the accounts as
click-to-fill buttons, and a banner says the data resets. The seed holds six
weeks of entries through now, a running entry for the admin and the member,
overlaps, a midnight-crossing entry, an archived project, a deleted entry and
plans for four weeks back to three weeks ahead; the audit log starts empty.

`--demo` only ever touches the `demo` workspace and refuses to start if the
database holds any other workspace, so never point it at a production database.
It also refuses the self-service signup flags, and its `robots.txt` disallows
crawling. Visitors share one workspace and its accounts: this is a demo, not a
Trial
([ADR-0015](https://github.com/havspect/tstamps/blob/main/docs/adr/0015-flat-subscription-read-only-suspension.md)),
and the reset is the only cleanup there is.

## Deploy a new build

After the first install, one command builds locally and ships an update with Ansible:

```sh
just deploy time.example.com
```

The first argument is the SSH host. The optional second and third arguments are the SSH user (default `root`) and the target architecture (default `amd64`):

```sh
just deploy time.example.com deploy arm64
```

`just deploy` runs `just build-prod linux <arch>`, then `ansible-playbook` against that host. It takes a database snapshot into `/var/backups/tstamps`, copies the binary, installs it with a rename while the old process runs, restarts the unit, and polls `/healthz` for up to 30 seconds. On failure it restores the previous binary and exits non-zero. The playbook uses only built-in modules, so there is no Galaxy download.

Install Ansible on the machine you deploy from. With mise: `mise use -g 'pypi:ansible-core[python=3.12]@latest'`. The `python=3.12` option matters because current ansible-core does not run on Python 3.15. Without mise, use `sudo pacman -S ansible` on Arch or `uv tool install --python 3.12 ansible-core`. The target must be set up with the steps above, and its SSH user needs passwordless `sudo`. Put keys, ports and usernames in `~/.ssh/config` and pass the alias as the host. Paths, the unit name and the `tstamps` service user are the defaults in `deploy/ansible/deploy.yml`; override them with `-e` when calling `ansible-playbook` directly.

The snapshot is taken before the restart, so a failed deploy keeps the data. If the new binary already migrated the schema, the previous binary refuses to start: restore the snapshot as described in [Backup and upgrade](./backup-upgrade.md#restore).

Legal pages are operator files under `/var/lib/tstamps/legal`, not part of the binary, so a deploy leaves them alone. See [Legal pages](./legal.md).

## Settings

Flags win over environment, which wins over the default.

| Flag | Environment | Default | Meaning |
| --- | --- | --- | --- |
| `--addr` | `TSTAMPS_ADDR` | `:8080` | Listen address |
| `--db` | `TSTAMPS_DB` | `./tstamps.db` | SQLite database file holding every workspace |
| `--legal-dir` | `TSTAMPS_LEGAL_DIR` | none | Directory holding `imprint.html` and `privacy.html`; see [Legal pages](./legal.md) |
| `--email-api-key` | `TSTAMPS_EMAIL_API_KEY` | none | Email API key; required for self-service signup |
| `--email-from` | `TSTAMPS_EMAIL_FROM` | none | Verified From address for signup mail |
| `--public-url` | `TSTAMPS_PUBLIC_URL` | none | Public origin used in emailed links |
| `--email-api-url` | `TSTAMPS_EMAIL_API_URL` | `https://api.resend.com/emails` | Email endpoint |
| `--demo` | `TSTAMPS_DEMO` | off | Seed the fictional demo workspace and reset it hourly |
| `--log-level` | `TSTAMPS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

```sh
tstamps-server serve --addr 127.0.0.1:8080 --db /var/lib/tstamps/tstamps.db --log-level warn
```

`workspace create` and `backup` also take `--db` (or read `TSTAMPS_DB`). None of them take `--addr`.

## Health and logs

`GET /healthz` answers `200` only while the database is reachable, and `503` when it cannot. It checks the process's database, not a particular workspace. Use it from a monitor:

```sh
curl -fsS http://127.0.0.1:8080/healthz
```

Each workspace has its own `/w/{slug}/healthz`, which answers `200` and reports the schema version stored in the database, `404` for an unknown slug and `503` when the database cannot answer. Monitor the root once and each workspace you care about. See [Workspaces](./workspaces.md#health).

Every API request writes one structured log line to stderr: method, path without query, status, duration, bytes, remote IP and, when authenticated, user id. The SPA and its assets are not logged.

## Security notes

- There is no CORS: the SPA is served from the same origin as the API.
- Mutations over the session cookie require a same-origin `Origin` header.
- Login is rate limited per IP in memory.
- Signup sends at most five verification emails per hour per IP, also in memory. Its POSTs require a same-origin `Origin`, its links are single use, and pending signups are rows in the database rather than workspaces.
- A session or token is looked up only in the workspace named by `/w/{slug}`; credentials never cross workspaces.
- Passwords are stored as argon2id hashes; tokens are stored as SHA-256 and shown once.
- Admin edits and a lead's project changes are recorded in the [audit log](./admin.md#audit-log).

## Next

- [Workspaces](./workspaces.md) — creation, self-service email signup, slugs and isolation.
- [Back up and upgrade](./backup-upgrade.md) the database.
- [Install the CLI](./install-cli.md) and point it at a workspace.
