# One workspace per install, shipped as one binary

Status: the one-workspace-per-install decision is superseded by [ADR-0019](0019-workspaces-share-one-sqlite-file.md) (through [ADR-0011](0011-host-workspaces-as-separate-sqlite-files.md), itself superseded). The "one binary" claim is amended by [ADR-0008](0008-user-cli-and-server-are-two-binaries.md) — the workspace still ships without a database, cache or frontend host, but client commands are now a separate `tstamps` artifact.

A workspace is one company. The Go binary serves the HTTP API, the embedded web UI and an embedded SQLite file; there is no separate database, cache or frontend host. Self-hosting is `tstamps serve --db ./tstamps.db`, and a backup is a file copy.

Considered multi-tenant SaaS (rejected: every table and query grows an org scope, and cross-tenant leaks are the expensive failure mode) and Postgres (rejected: kills the single-binary install for a team-sized product).

Consequence: one server process per workspace, SQLite's single-writer limit sets the ceiling, and horizontal scaling is out of scope by design.
