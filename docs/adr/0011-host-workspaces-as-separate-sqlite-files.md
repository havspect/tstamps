# Host workspaces as separate SQLite files

Status: superseded by [ADR-0019](0019-workspaces-share-one-sqlite-file.md). Hosted mode now keeps every workspace in one SQLite file, with `workspace_id` scoping rows. The design below is kept for history. The per-workspace files, workspace-local users and operator-run backups were the state before ADR-0019; suspension, deletion and billing remain outside this design.

A hosted server may run multiple workspaces, but each workspace keeps its own SQLite file and workspace-local users. This preserves the existing schema, queries, authorization and per-workspace backup boundaries, and avoids making a missing workspace predicate a cross-company data leak. The existing **Admin** role remains workspace-local; operators create workspaces and their first admins through the server CLI, so there is no Server Admin or remote control plane.

Hosted mode kept one SQLite file per workspace and selected workspaces through `/w/{slug}/...`; the slug is an opaque operator-assigned identifier, not a DNS domain. `--db` installations were one-workspace installations with unchanged unprefixed URLs. CLI and MCP clients carry the workspace in their configured base URL. A browser has one workspace login at a time per server.

Considered options: one shared database (rejected: every model, lookup and constraint would need workspace scoping); global users belonging to multiple workspaces (rejected: couples identities and credentials across companies); remote workspace administration (rejected as unnecessary for operator-managed hosting).

Consequences: hosted backups contain one independently consistent snapshot per workspace, not one atomic cross-workspace snapshot. Workspace creation and all-workspace backup are operator commands; suspension, deletion workflows and billing are outside this decision. Self-service signup was later added within the same per-workspace boundary ([ADR-0014](0014-self-service-workspace-signup.md)).
