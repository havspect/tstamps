# Admin console

Admins manage one workspace under `/w/{slug}/admin`: **Users**, **Teams**, **Projects** and **Entries**. The screens are tables with inline create and edit. There is no separate settings page, and no screen reaches another workspace.

The first admin of each workspace is created from the server host, not the web UI: `tstamps-server workspace create <slug> --email x --name y` on the same database. See [Server on Linux](./server.md) and [Hosted workspaces](./workspaces.md). After that, everything below happens in the browser.

## Users

**Admin → Users** lists every user with email, name, timezone, admin flag and active state.

**Create user** asks for email, name, timezone and whether the user is an admin. The console shows an **initial password** once. Copy it before dismissing the banner; it is not shown again.

A new or reset user carries `must_change_password`: they can sign in, but every API call except `/me`, `/me/password` and `/auth/logout` is refused until they replace it in the web UI. The CLI cannot mint a token before that.

**Edit** changes name, timezone, admin flag and active state.

- Deactivating a user stops their running entry at that instant, with the note `stopped on deactivation`. Their hours stay in every report.
- The last active admin cannot be demoted or deactivated (`422 last-admin`).

**Reset password** issues a new initial password and ends every existing login for that user: sessions are deleted and all API tokens are revoked. They must change the password at next sign-in.

## Teams

**Admin → Teams** lists teams, their member count and whether they are archived.

- **Create** adds a team.
- **Rename** changes its name.
- **Members** replaces the team's membership list with the users and roles you choose (`lead` or `member`).
- **Archive** archives the team and every project it owns in the same step. The membership rows stay but grant nothing.

::: warning Archiving a team is permanent in v1
There is no team restore. Its projects cannot be restored either while the team is archived. Hours already logged remain in reports. Archive only when you mean it.
:::

A project's team can be changed while the project has no entries. Once it has any, the team is fixed (`422 project-has-entries`).

## Projects

**Admin → Projects** lists projects with their team and status.

- **Create** asks for a name, a team and a colour.
- **Archive** hides the project from pickers but keeps its hours in reports as `(archived)`.
- **Restore** brings an archived project back. It is refused while the owning team is archived.

Leads can create and archive projects for their own teams via the CLI and API; the admin screen shows all of them. See [CLI reference](./cli.md#projects-and-teams).

## Deleted entries

Deleting an entry from the timeline or with `tstamps rm` sets `deleted_at`. The entry disappears from lists and totals but stays in the database.

**Admin → Entries** has two tabs: live and deleted, over a date range. **Restore** clears `deleted_at` and the entry counts again.

Nothing in the workspace is hard-deleted. Restoring needs no backup.

## Audit log

An audit event records a change where your authority over the record came from a role: an admin acting on the workspace, or a lead acting on their team's projects. A person editing their own entry writes nothing. That is the product working as designed, not a privilege being spent.

Each event keeps who acted and the role they used (`admin` or `lead`), when, the record it touched (`project` `7`), the action name (`project.archive`), and the audited fields before and after. Password and token hashes never appear; a password reset records that it happened. One change can write several events: archiving a team writes one `project.archive` per archived project, and deactivating a person writes the `entry.stop` that ends their running entry.

Read it as an admin, newest first:

```sh
tstamps audit --action user.update --from 2026-10-01 --to 2026-11-01
tstamps audit --target-type project --target-id 7 --limit 50
tstamps audit --actor sam@example.com
```

That answers "who deactivated Sam?" and "what happened to project 7 last week?". Rows come back newest first, 200 by default and at most 1000 with `--limit`. Filters are `--from`/`--to` (dates, `from` inclusive, `to` exclusive), `--actor EMAIL`, `--action NAME`, `--target-type TYPE`, `--target-id N`, and `--json`. The API is `GET /w/{slug}/api/v1/audit`; there is no admin screen and no MCP tool.

Limits worth knowing:

- It starts empty on the day this version ships; nothing older is backfilled.
- It is per workspace. Each workspace has its own log and there is no combined view.
- It is append-only: no delete path, no retention job. Events stay for the life of the database.
- An operator who owns the database file can edit it. It protects against the application, not the operator; a backup taken before an incident is the stronger evidence.
- It is not a history of hours. What a person does to their own entry — logging, retiming, deleting — is not an event. The two exceptions are an admin restoring a deleted entry, and the stop that deactivation causes.
