# Plan a person only onto their own teams' projects

Status: accepted, 2026-11-01.

## Context

Writing a plan cell checks two things separately: the caller may manage the project's team (`ActionManagePlan`), and the caller may plan the person. Nothing ties the person to the project. An admin could plan a Product person onto a Platform project, and the cell was stored. The person's Product lead then saw it as unnameable "other teams" hours: a load on their row that nobody on their team could see, name or change.

There is no project-membership or bookable-project concept in the schema, and inventing one would be a second, unrelated notion of who may work on what.

## Decision

A plan cell may only name a project whose team is one of the planned person's teams. The rule holds for admins too. A write that breaks it fails with `422` and the detail "the project is not in this person's teams".

Two exceptions keep the data manageable:

- Zero hours still deletes an existing cell that breaks the rule, so cells written before it are not frozen in place.
- Seed and repeat skip such cells instead of writing them, and count them as skipped.

Cells that already break the rule stay visible and readable; there is no migration.

## Why

- A person's teams are where their lead and their capacity live. A cell outside those teams is a number on someone's row that their lead cannot read, which is worse than refusing the write.
- Checking the person's teams — data the plan service already loads — needs no new table, no new field and no migration.
- Refusing admins too is deliberate. The rule is about what a cell means, not about who is trusted; an admin who needs the hours planned can add the person to the team first, which is the decision they were making anyway.

## Considered options

- **Project membership or a bookable flag on projects** — rejected: a second membership model to keep in step with teams, for a rule teams already express.
- **Warn or label the offending cell instead of refusing** — rejected: it leaves a number the person's lead can neither name nor change.
- **Refuse zero-hour deletes as well** — rejected: it freezes every pre-existing cross-team cell forever.

## Consequences

- The project picker in the person view lists only projects of that person's teams; the project view is unchanged, because it lists the project's own team's members.
- Seed and repeat silently lose cross-team pairs their source data named; they land in the skipped count.
- Existing cross-team cells render and read like archived ones: visible, labelled, no longer writable except to zero.
- No schema change, no migration.
