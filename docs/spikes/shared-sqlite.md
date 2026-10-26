# Spike: workspaces as rows in one SQLite file

Status: accepted, implemented, 2026-10-25. See
[ADR-0019](../adr/0019-workspaces-share-one-sqlite-file.md). Reverses the storage
choice in [ADR-0011](../adr/0011-host-workspaces-as-separate-sqlite-files.md) and
folds the host database that [ADR-0016](../adr/0016-server-level-operator.md) and
[ADR-0015](../adr/0015-flat-subscription-read-only-suspension.md) planned into
the same file. Nothing was deployed under the per-file layout, so there is no
migration.

## Question

Can every workspace live in the one database `--db` already opens, with a
`workspace_id` column, instead of a separate database file for each? Would that
simplify v2? How much work is it?

## What the spike does

- `workspaces` table: `id`, `slug`.
- `workspace_id` on users, sessions, teams, memberships, projects, entries,
  audit events and API tokens.
- `store.Store` keeps its public API but is bound to one workspace. Every query
  goes through `s.scoped()`, every insert stamps `workspace_id`. All database
  access in the repo was already inside `internal/store`, so handlers, reports,
  entry rules, CLI, MCP and the SPA are unchanged.
- `store.Registry` owns the one pool: `WorkspaceBySlug`, `Workspaces`,
  `CreateWorkspace`, `ForWorkspace(id)`, one `Migrate` and one `BackupTo`.
- `workspace.Host` routes every request under `/w/{slug}` and caches one handler
  per slug. There is no second storage mode and no unprefixed workspace route.
- `tstamps-server workspace create <slug> --db path` is the only bootstrap. It
  works on a fresh or an existing database; each call adds a row and its first
  admin in one transaction.
- `GET /login` redirects to `/w/{slug}/login` when the database holds exactly
  one workspace, and shows the slug prompt when it holds more.
- Unique indexes are composite (`workspace_id, email`), and the running-entry
  partial index is per workspace.
- Pending self-service signups are rows in the same database: token hash, email,
  created at. There is no directory of pending link files. A row is claimed by
  deleting it, so one link creates one workspace.
- Schema 7. There is no upgrade path from the per-file schema or a schema-5
  `--db` database; nothing was deployed, so local databases are recreated.

Size: see `git diff --stat`; the core is the store scope, the registry and the
host. 53 `Store` methods now carry the predicate.

## Evidence

`go test ./...`, `go vet ./...` and `-race` on `store`/`workspace` pass,
including:

- `TestRegistryScopesEveryWorkspace`: same email in two workspaces; reads by
  another workspace's user/project/team/entry/session/token/audit IDs return
  `ErrNotFound`; writes (password reset, save, archive, memberships) are
  refused; one running entry per workspace is legal; both workspaces are rows
  in one file.
- `TestHostIsolatesWorkspaces`: a cookie or token from one `/w/{slug}` is
  rejected by the other.
- `TestHostKeepsRowsOfEachWorkspaceApart`: projects and users listed through one
  workspace contain only its rows, and an entry or project ID from the other
  workspace answers `404` on update or restore.
- `/login` redirects on a one-workspace database and prompts on a two-workspace
  one.
- CLI: two workspaces with the same admin email, one `backup` command produces
  a single snapshot containing both.

## What gets simpler

1. **v2 loses `server.db`.** Billing state, trial/suspension, operator accounts
   and audit, and workspace rows live where the workspace lives. No adoption of
   unregistered directories on boot, no metadata reads fanned out per file, no
   two-file backup.
2. **Suspension is one transaction.** Set state and stop every running entry
   (`UPDATE entries ... WHERE workspace_id = ?`) atomically.
3. **Backups are one point in time.** `VACUUM INTO` gives one consistent file
   for every workspace. Restore-all is one file.
4. **One migration at boot.** Every workspace shares one version check and one
   `AutoMigrate`; nothing waits for its first request.
5. **Workspace creation is atomic.** One transaction replaces staging a file and
   linking it, and it works on an existing database.
6. **One connection pool, one WAL.** One process serves every workspace without
   caching a store and pool per slug forever.
7. **One storage shape.** `--db` is not a single-workspace variant; it is the
   whole server, and a second workspace is another row.
8. **Pending signups are rows.** No pending directory, no backup exception,
   and a link is claimed by one delete.

## What gets worse

1. **Every query needs the predicate.** One forgotten `workspace_id` is a
   cross-company leak, which is why ADR-0011 rejected this. The blast radius is
   contained: all SQL sits behind `Store`, and a scoped handle plus one
   isolation test catches the common mistakes. It is still an invariant every
   future query must respect, so the test must cover the whole API surface, not
   just login and tokens.
2. **One writer for the whole server.** SQLite allows one write transaction at
   a time. One company's write can now block another's, up to the 10s busy
   timeout. Fine for a team-sized server on one VM; it is the main capacity
   ceiling to accept.
3. **Blast radius.** One corrupt or lost file takes every workspace with it.
   Backups become more important, not less.
4. **Per-workspace restore is no longer `cp`.** Whole-file restore is. Exporting
   or restoring one workspace needs its own command; deferred, because nothing
   is deployed and there is no customer asking for it yet.
5. **No migration is written.** Nothing was deployed under the per-file layout
   or schema 5, so the new schema starts clean.
6. **Migration discipline.** Every new tenant table needs `workspace_id` and
   composite unique indexes; partial unique indexes remain handwritten SQL.
7. **Global IDs** expose rough cross-tenant volume (a user can see IDs are not
   contiguous). Cosmetic; per-workspace IDs would mean composite keys.
8. **Deletion** becomes a multi-table transaction instead of `rm`; keep it in
   the CLI, one irreversible transaction.

## Effort

| Piece | Result |
| --- | --- |
| Schema + scoped store + registry + `/w` host | done |
| One `--db` path, `workspace create` bootstrap, `/login` redirect | done |
| Pending signups as rows | done |
| Existing tests adapted | done, all green |
| Cross-workspace store and API isolation tests | done |
| Docs, ADR reversal, web copy | done |
| Legacy importer / per-workspace export-restore | deliberately not written: nothing is deployed |
| v2 operator/billing tables on the same file | afterwards, *less* work than the per-file plan |

The architecture moved cheaply because every query already went through
`internal/store`. The lasting cost is the isolation invariant, not the diff.

## Decision

Accepted in [ADR-0019](../adr/0019-workspaces-share-one-sqlite-file.md): adopt it
before building the v2 host database. Preconditions kept:

1. Keep the per-workspace `Store` boundary; no handler or report gets a raw
   `*gorm.DB`.
2. Keep an API-level test that seeds two workspaces and asserts each one's IDs
   are invisible to the other.
3. Keep one database and one server shape: every workspace under `/w/{slug}`,
   no second storage flag.
4. Decide the single-writer budget (workspaces per server) and document it.
5. Record the reversal of ADR-0011, and fold ADR-0015/0016's `server.db` into
   the same file.

## How to try the spike

```sh
go test ./internal/store ./internal/workspace ./...
go run ./cmd/tstamps-server workspace create alpha --db ./tstamps.db --email a@example.com --name A
go run ./cmd/tstamps-server workspace create beta  --db ./tstamps.db --email a@example.com --name A
go run ./cmd/tstamps-server backup --db ./tstamps.db ./snap.db
go run ./cmd/tstamps-server serve --db ./tstamps.db
curl -fsS http://127.0.0.1:8080/w/alpha/healthz
sqlite3 ./tstamps.db 'select id, slug from workspaces; select workspace_id, email from users;'
```
