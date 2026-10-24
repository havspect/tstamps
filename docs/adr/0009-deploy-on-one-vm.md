# Production runs on one VM: systemd and Caddy, no container

Status: one process may now host several workspaces in hosted mode ([ADR-0011](0011-host-workspaces-as-separate-sqlite-files.md)); the one-VM systemd and Caddy deployment is unchanged.

A workspace is deployed as the `tstamps-server` binary on one VM, under systemd, behind Caddy on the same host. Caddy terminates TLS and proxies to `127.0.0.1:8080`; systemd owns the process, the data directory and the nightly backup timer. The unit files and Caddyfile are in `deploy/`.

Why: ADR-0001 already accepts one process per workspace and SQLite's single writer, so there is no availability to buy with replicas. A container would add a volume, a probe distinction and a backup sidecar; Kubernetes would add all of that plus a scheduler for a workload that must run exactly once. systemd already provides restart, sandboxing, a state directory and timers, and Caddy provides certificates and HSTS from four lines.

Consequences: the server binds localhost and must never be reachable directly, because it trusts `X-Forwarded-Proto` to decide whether it can set the `__Host-` cookie, and the last `X-Forwarded-For` hop for client addresses in logs and the login rate limiter. Both assumptions hold only while the proxy is the sole way in. There is no container image, `.deb` or chart; installation is the binary, three unit files and a Caddyfile. Backups are snapshots on the same disk and are only a backup once the operator copies them off the VM with their own tool. Moving to containers later means re-deciding the proxy trust and backup hooks, not rewriting the server.
