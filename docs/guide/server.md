# Server on Linux

Tstamps server runs on Linux. The CLI runs on Linux, macOS and Windows; the server does not.

One `tstamps-server` process serves the web UI, the REST API and its SQLite storage: one database file with `--db`, or one file per workspace with `--workspace-dir`. The UI is embedded in the binary, so the server is built on the machine that runs it. There is no server release archive.

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

For API work, `just build` leaves symbols in and is faster. A plain `go build ./cmd/tstamps-server` also works but serves a "UI is not built" page instead of the app.

## Run it locally first

```sh
tstamps-server admin create --email you@example.com --name "Your Name" --db ./tstamps.db
tstamps-server serve --addr :8080 --db ./tstamps.db
```

The first command prints an initial password. Open `http://localhost:8080`, sign in, change the password when prompted, then create teams, users and memberships under **/admin**. The CLI reads those; it does not create them.

`admin create` opens the database directly. It works while the server is running, which is also how you bootstrap the first admin on a deployed instance.

## Host several companies

Hosted mode keeps one SQLite file per workspace and selects it by slug in the URL. Create the first workspace, then serve the directory:

```sh
tstamps-server workspace create example.com --workspace-dir ./workspaces \
  --email you@example.com --name "Your Name"
tstamps-server serve --addr :8080 --workspace-dir ./workspaces
```

The first command prints the workspace admin's initial password and creates the directory if needed. Open `http://localhost:8080/`, enter `example.com`, and sign in. `--db` and `--workspace-dir` are mutually exclusive. The full shape is on [Hosted workspaces](./workspaces.md).

To let visitors create their own workspace, add `--email-api-key`, `--email-from` and `--public-url`. Without them, signup is off. Details and the email setup are on [Hosted workspaces](./workspaces.md#self-service-signup).

## Deploy on a VM

The production shape is one VM running `tstamps-server` under systemd, with Caddy terminating TLS on the same host. The unit files and Caddyfile are in [`deploy/`](https://github.com/havspect/tstamps/tree/main/deploy). The server binds loopback; Caddy is the only public listener.

```sh
# After `just build-prod` and copying tstamps-server to the VM:
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

Create the first admin against the live database:

```sh
sudo -u tstamps tstamps-server admin create \
  --db /var/lib/tstamps/tstamps.db --email you@example.com --name "Your Name"
```

For hosted mode, put `--workspace-dir /var/lib/tstamps/workspaces` in the unit instead of `--db`, create each workspace with `sudo -u tstamps tstamps-server workspace create ... --workspace-dir /var/lib/tstamps/workspaces`, and start the service once the directory exists. There is no server-wide admin: each workspace's first admin comes from its own `workspace create`.

Caddy sets `X-Forwarded-Proto`, which is how the server knows to mark the session cookie `Secure`. It also sets `X-Forwarded-For`, the hop used for client IPs in logs and for login rate limiting. The server trusts both only from a loopback peer, so a direct client cannot forge them. Keep port 8080 off the network anyway.

## Settings

Flags win over environment, which wins over the default.

| Flag | Environment | Default | Meaning |
| --- | --- | --- | --- |
| `--addr` | `TSTAMPS_ADDR` | `:8080` | Listen address |
| `--db` | `TSTAMPS_DB` | `./tstamps.db` | One SQLite file |
| `--workspace-dir` | `TSTAMPS_WORKSPACE_DIR` | none | Directory of workspace databases (hosted mode) |
| `--email-api-key` | `TSTAMPS_EMAIL_API_KEY` | none | Email API key; required for self-service signup |
| `--email-from` | `TSTAMPS_EMAIL_FROM` | none | Verified From address for signup mail |
| `--public-url` | `TSTAMPS_PUBLIC_URL` | none | Public origin used in emailed links |
| `--email-api-url` | `TSTAMPS_EMAIL_API_URL` | `https://api.resend.com/emails` | Email endpoint |
| `--log-level` | `TSTAMPS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

Set one storage mode: `--db` or `--workspace-dir`, never both.

```sh
tstamps-server serve --addr 127.0.0.1:8080 --db /var/lib/tstamps/tstamps.db --log-level warn
```

`admin create` takes `--db`; `workspace create` and `backup` take `--workspace-dir` (and `backup` also takes `--db`). None of them take `--addr`.

## Health and logs

`GET /healthz` answers `200` only while SQLite is reachable, and reports the schema version stored in the file. It answers `503` when it cannot. Use it from a monitor:

```sh
curl -fsS http://127.0.0.1:8080/healthz
```

In hosted mode the root `/healthz` answers `200` while the workspace directory is readable, without opening a database, and each workspace has its own `/w/{slug}/healthz` that reports the schema version in that file. Monitor the root once and each workspace you care about. See [Hosted workspaces](./workspaces.md#health).

Every API request writes one structured log line to stderr: method, path without query, status, duration, bytes, remote IP and, when authenticated, user id. The SPA and its assets are not logged.

## Security notes

- There is no CORS: the SPA is served from the same origin as the API.
- Mutations over the session cookie require a same-origin `Origin` header.
- Login is rate limited per IP in memory.
- Signup sends at most five verification emails per hour per IP, also in memory. Its POSTs require a same-origin `Origin`, its links are single use, and pending signups are `0600` files under the workspace directory rather than workspaces.
- In hosted mode a session or token is looked up only in the workspace named by `/w/{slug}`; credentials never cross databases.
- Passwords are stored as argon2id hashes; tokens are stored as SHA-256 and shown once.
- Admin edits and a lead's project changes are recorded in the [audit log](./admin.md#audit-log).

## Next

- [Host several companies](./workspaces.md) on this server.
- [Back up and upgrade](./backup-upgrade.md) the workspace.
- [Install the CLI](./install-cli.md) and point it at this server.
