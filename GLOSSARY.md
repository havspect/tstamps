# Tstamps

Team time tracking. People log intervals of work against projects, team leads read the hours logged on their projects, admins run the workspace.

## Language

**Workspace**:
One company's users, teams, projects and entries. A server runs one or more workspaces.
_Avoid_: tenant, organisation, org, account

**Workspace slug**:
The unique identifier used to select a workspace, chosen by an operator or by the person signing up. It is not a DNS domain, even when it looks like one.
_Avoid_: workspace ID, subdomain

**Operator**:
The person who runs the server and the workspaces on it. Not a User: an Operator is a member of no workspace, logs no entries, and acts through the server CLI and the operator console.
_Avoid_: server admin, superuser, root, sysadmin

**Signup**:
The self-service way to create a workspace when the operator has enabled email verification: an email owner verifies the address, then chooses the workspace slug, their name and a password. Contrasts with an operator's `workspace create`.
_Avoid_: registration, onboarding, tenant creation

**Pending signup**:
An emailed link that has been requested but not yet used to create a workspace. It holds one email address, not a workspace, and it never expires.
_Avoid_: invite, request, application

**Trial**:
The first 14 days of a self-service workspace, before the first payment. It has every feature; when it ends the workspace becomes suspended, it is not deleted.
_Avoid_: free plan, beta, free tier

**Suspended workspace**:
A workspace whose trial ended or whose payment failed. Everyone may sign in, read and export; logging and admin changes are refused. Its data stays until the Operator deletes the workspace.
_Avoid_: locked, disabled, dormant, frozen

**Billing contact**:
The address that receives a workspace's invoices. Not necessarily a User, and not an authority: any admin may change it.
_Avoid_: payer, owner, account holder

**User**:
A person who can log in and log hours in one workspace. The same person in another workspace is a separate User.
_Avoid_: account, employee, member (see **Member**)

**Admin**:
A user who may manage users, teams, projects, and restore deleted data.
_Avoid_: owner, superuser

**Team**:
A named group of users that owns projects and has one or more leads. A user may belong to several teams.
_Avoid_: group, department, squad

**Lead**:
A membership role on a team. Leads create and maintain that team's projects and read the hours logged on them.
_Avoid_: manager, owner, team lead (as a noun for the person; "lead" is fine)

**Member**:
A membership role with no extra rights: log own hours, read own totals.
_Avoid_: employee, developer, user

**Membership**:
The link between a user and a team, carrying the role of lead or member.
_Avoid_: seat, assignment

**Initial password**:
The password an admin sets for a new or reset user, which the user must replace at first login. A password chosen during self-service signup is not an initial password.
_Avoid_: temporary password, one-time password, OTP

**Deactivated user**:
A user who can no longer log in; their hours stay in reports.
_Avoid_: deleted user, disabled user

**Personal token**:
A user-created bearer credential with the user's full access in one workspace, used by scripts and local tools until revoked.
_Avoid_: API key, pat, static token

**OAuth client**:
An application — the CLI, the mobile app, or an MCP agent — that requests scoped access to a workspace on a user's behalf.
_Avoid_: app, integration, consumer, service account

**Grant**:
A user's revocable authorization letting an OAuth client act on their behalf in one workspace with specific scopes.
_Avoid_: consent record, connected app, OAuth session

**Identity provider**:
The external OIDC service a workspace trusts to prove who a user is. It proves identity only: it grants no team, role or workspace access.
_Avoid_: SSO server, auth server, login provider

**External identity**:
The stable link between a User and their issuer and subject at the workspace's Identity provider. The first login may adopt a User by verified email; later logins follow this link rather than treating email as the identifier.
_Avoid_: SSO account, email identity, linked account

**SSO**:
Signing in through the workspace's Identity provider instead of a workspace password. Once enforced, only the Break-glass admin still uses a password and TOTP.
_Avoid_: enterprise login, social login

**Break-glass admin**:
The one workspace admin who can still sign in with a password and a TOTP code after SSO is enforced, for IdP outages and lockouts.
_Avoid_: backup admin, emergency account, super admin

**Project**:
The unit of work that hours are logged against. Belongs to exactly one team and is visible to everyone in the workspace.
_Avoid_: task, ticket, job, client, engagement

**Entry**:
One user's continuous stretch of work on one project, with a start, an end and an optional note.
_Avoid_: record, timesheet, booking, time log

**Running entry**:
An entry with a start but no end yet. A user has at most one.
_Avoid_: timer, active entry, open record

**Log**:
To record an entry against a project.
_Avoid_: book, track, record (as a verb)

**Timeline**:
The personal, horizontally scrollable view of one user's entries over time, where logging happens.
_Avoid_: canvas, calendar, chart, board

**Note**:
Free text on an entry describing what was done.
_Avoid_: description, comment, remark

**Week**:
An ISO week: Monday to Sunday, labelled like `2026-W43`.
_Avoid_: calendar week, pay week

**Month**:
A calendar month in the entry's timezone, January to December, labelled like `2026-10`.
_Avoid_: period, cycle, pay period

**Bucket**:
One column of a report: a day in an entry's timezone, an ISO week, or a calendar month.
_Avoid_: period, column, interval

**Report**:
Totals of logged time, grouped by project and by person, over days, weeks or months.
_Avoid_: dashboard, stats, analytics

**Timezone**:
A user's own IANA zone. A default for undated queries and display; it does not decide which day an entry falls into.
_Avoid_: locale, region

**Entry timezone**:
The IANA zone an entry was logged in. Decides which day and week that entry's hours fall into. Changing a user's timezone does not move old entries.
_Avoid_: offset, local time

**Archived project**:
A project hidden from project pickers whose logged hours still count in reports. Only an admin restores it.
_Avoid_: deleted project, inactive project

**Deleted entry**:
An entry removed from all lists and totals but kept in storage. Only an admin restores it.
_Avoid_: archived entry, voided entry

**Audit event**:
A record of one change made by a user whose authority over the target comes from a role, not from ownership: an admin, or a lead acting on their team's projects. A user changing their own entry is not an audit event.
_Avoid_: history, changelog, activity, version

**Audit log**:
The append-only list of audit events, read by admins. It is not a feed and not a backup.
_Avoid_: history, activity feed, changelog
