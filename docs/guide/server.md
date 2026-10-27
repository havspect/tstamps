# Server on Linux

Tstamps server runs on Linux. The CLI runs on Linux, macOS and Windows; the server does not.

One `tstamps-server` process serves the web UI, the REST API and its SQLite storage: one database file holding every workspace at `/w/{slug}/...`. The UI is embedded in the binary, so the server is built on the machine that runs it. There is no server release archive.

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

Create the first workspace against the live database:

```sh
sudo -u tstamps tstamps-server workspace create example.com \
  --db /var/lib/tstamps/tstamps.db --email you@example.com --name "Your Name"
```

There is no server-wide admin: each workspace's first admin comes from its own `workspace create`. Add further workspaces the same way, with a different slug and email.

Caddy sets `X-Forwarded-Proto`, which is how the server knows to mark the session cookie `Secure`. It also sets `X-Forwarded-For`, the hop used for client IPs in logs and for login rate limiting. The server trusts both only from a loopback peer, so a direct client cannot forge them. Keep port 8080 off the network anyway.

## Settings

Flags win over environment, which wins over the default.

| Flag | Environment | Default | Meaning |
| --- | --- | --- | --- |
| `--addr` | `TSTAMPS_ADDR` | `:8080` | Listen address |
| `--db` | `TSTAMPS_DB` | `./tstamps.db` | SQLite database file holding every workspace |
| `--email-api-key` | `TSTAMPS_EMAIL_API_KEY` | none | Email API key; required for self-service signup |
| `--email-from` | `TSTAMPS_EMAIL_FROM` | none | Verified From address for signup mail |
| `--public-url` | `TSTAMPS_PUBLIC_URL` | none | Public origin used in emailed links |
| `--email-api-url` | `TSTAMPS_EMAIL_API_URL` | `https://api.resend.com/emails` | Email endpoint |
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
