# Tstamps

Team time tracking. People log intervals of work against projects, team leads read the hours logged on their projects, admins run the workspace.

## Language

**Workspace**:
One company's users, teams, projects and entries. One server binary serves one workspace today; hosting several per server is planned ([ADR-0011](docs/adr/0011-host-workspaces-as-separate-sqlite-files.md)).
_Avoid_: tenant, organisation, org, account

**Workspace slug**:
The unique, operator-assigned identifier that will select a workspace once a server hosts several. It is not a DNS domain, even when it looks like one.
_Avoid_: workspace ID, subdomain

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
The password an admin sets for a new or reset user, which the user must replace at first login.
_Avoid_: temporary password, one-time password, OTP

**Deactivated user**:
A user who can no longer log in; their hours stay in reports.
_Avoid_: deleted user, disabled user

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
