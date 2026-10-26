# Workspaces share one SQLite database

Status: accepted, 2026-10-25. Supersedes [ADR-0011](0011-host-workspaces-as-separate-sqlite-files.md).
Amends [ADR-0014](0014-self-service-workspace-signup.md), whose signup now
creates a row, and folds the host database planned by
[ADR-0015](0015-flat-subscription-read-only-suspension.md) and
[ADR-0016](0016-server-level-operator.md) into the same file. No per-file
installation exists yet, so there is deliberately no in-place data migration.

A server keeps every workspace in the SQLite database named by `--db` (default
`./tstamps.db`). There is one storage mode: no workspace directory, no second
database. A `workspaces` table names the workspaces; every tenant row carries
`workspace_id`. `tstamps-server workspace create <slug> --db path ...` creates
a workspace and its first admin in one transaction; it is the only bootstrap.

Every workspace is addressed as `/w/{slug}/...`, including the single-company
case. The slug is selected before authentication, and authentication only sees
that workspace's users, sessions and tokens. When the database holds exactly one
workspace, `/login` redirects to `/w/{slug}/login`, so the user is never asked
to type a workspace name. With two or more, `/login` shows the prompt and each
workspace stays under its own prefix. Pending self-service signups are rows in
the same database, keyed by token hash, not files in a directory.

`store.Store` stays the public unit: one store is bound to one workspace and
every query and insert carries its `workspace_id`. `store.Registry` owns the
shared connection pool and the workspace list, and `workspace.Host` looks a slug
up there and caches one handler per slug. All SQL already lives behind
`internal/store`, so handlers, reports, the entry service, the CLI, MCP and the
SPA are unchanged.

Why one file:

- v2's operator console, trial/suspension state, billing mapping and operator
  audit all need to read or change workspace state. One database makes that one
  transaction; the per-file design needed a separate `server.db`, boot-time
  adoption of unregistered directories, metadata reads fanned out per file and
  two things to back up consistently.
- Backup becomes one `VACUUM INTO` snapshot that is a point in time for every
  workspace, instead of N individually consistent files. Restore-all is one
  file.
- Migrations run once at boot, not on the first request to each workspace.
- Creating a workspace and its first admin is one transaction, not a staged
  file plus a hard link.
- One connection pool replaces one cached pool per workspace, with no eviction
  today.
- There is one storage shape and one URL shape: a one-company install and a
  ten-company install differ only in how many workspace rows exist.

The cost, recorded plainly: every query must carry the workspace predicate, and
a forgotten one is a cross-company leak. That was ADR-0011's central objection
and it remains true. The containment is the `Store` boundary: no handler,
report or CLI code gets a `*gorm.DB`, every tenant model has `WorkspaceID`, and
an API-level test seeds two workspaces and asserts that each one's IDs are
invisible to the other. A new query method that forgets the scope is a review
failure, not a routine mistake.

Other consequences:

- SQLite now has one writer for the whole server. A busy workspace can block
  another for up to the 10s busy timeout. Per-file storage had per-workspace
  writers; this is the capacity trade the paid service accepts. Transactions
  begin with `_txlock=immediate` so a read-then-write waits instead of failing
  with `SQLITE_BUSY_SNAPSHOT`.
- One corrupt or lost file takes every workspace with it. Backups and the
  off-box copy matter more.
- Restoring one workspace is no longer `cp`. Whole-database restore is; per
  workspace export/restore is deferred until there is a customer who needs it.
- IDs are global, so a tenant can infer that other rows exist from gaps. A
  composite `(workspace_id, id)` key would avoid that at a large cost in every
  query and index; not worth it.
- The pre-shared schema cannot be upgraded in place because `workspace_id` has
  no column default. Nothing is deployed, so local databases are recreated.
  `SchemaVersion` is 7.

Considered and rejected: keeping per-file workspaces and adding `server.db`
(two sources of truth, non-atomic backup, fan-out reads); one SQLite file plus
`ATTACH` per workspace (still N files and still not one snapshot); composite
primary keys (a much larger change for a cosmetic metadata leak); PostgreSQL
(kills the single-binary install); unprefixed URLs for a single workspace
(a second workspace would move the first company's URLs; the login redirect
already removes the prompt without changing the URL shape).
