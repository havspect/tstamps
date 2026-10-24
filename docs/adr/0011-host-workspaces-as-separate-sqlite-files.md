# Host workspaces as separate SQLite files

Status: proposed, not implemented — the current binary still serves one workspace per process.

A hosted server may run multiple workspaces, but each workspace keeps its own SQLite file and workspace-local users. This preserves the existing schema, queries, authorization and per-workspace backup boundaries, and avoids making a missing workspace predicate a cross-company data leak. The existing **Admin** role remains workspace-local; operators will create workspaces and their first admins through the server CLI, so there is no Server Admin or remote control plane.

Hosted mode will use `--workspace-dir` and select workspaces through `/w/{slug}/...`; the slug is an opaque operator-assigned identifier, not a DNS domain. Existing `--db` installations remain one-workspace installations with unchanged unprefixed URLs. CLI and MCP clients will carry the workspace in their configured base URL. A browser will have one workspace login at a time per server.

Considered options: one shared database (rejected: every model, lookup and constraint would need workspace scoping); global users belonging to multiple workspaces (rejected: couples identities and credentials across companies); remote workspace administration (rejected as unnecessary for operator-managed hosting).

Consequences: hosted backups contain one independently consistent snapshot per workspace, not one atomic cross-workspace snapshot. Workspace creation and all-workspace backup are operator commands; suspension, deletion workflows, self-service signup and billing are outside this decision.
