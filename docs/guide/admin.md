# Admin console

Admins manage the workspace under `/admin`: **Users**, **Teams**, **Projects** and **Entries**. The screens are tables with inline create and edit. There is no separate settings page.

The first admin is created from the server host, not the web UI: see [Server on Linux](./server.md). After that, everything below happens in the browser.

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

## What is not audited

Admin edits are not audited in v1. That is an accepted risk, recorded in [the spec](https://github.com/havspect/tstamps/blob/main/docs/SPEC.md), not a missing setting.
