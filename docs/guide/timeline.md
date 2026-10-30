# Timeline

Open `/w/{slug}/timeline` after signing in. The timeline is one day, one column per project, hours running down the page. It is where most people log.

![The timeline: one column per project, one day of entries](/screenshots/timeline.webp)

## Log an entry

| To do this | Do that |
| --- | --- |
| Create an entry | Drag on an empty part of a project column |
| Retime an entry | Drag its top or bottom edge |
| Move an entry | Drag its body |
| Edit project or note | Click the entry |
| Start a timer | Press the play button in a project header, or press `space` |
| Stop the timer | Press the square in the running project's header, or press `space` |
| Change day | Use the arrows, or **Today** |
| Add a project column | Press **+** and pick a project |

Entries snap to 15 minutes. Hold `Alt` while dragging for one-minute precision.

The default note is empty. Notes are free text; they are not parsed and never affect reports.

## Keyboard

| Key | Action |
| --- | --- |
| `space` | Start or stop the running entry on the current or last-used project |
| `n` | New one-hour entry ending now |
| `Delete` / `Backspace` | Delete the selected entry, after a confirmation |
| `Esc` | Cancel a drag or close the edit panel |

`space`, `n` and `Delete` are ignored while a form control or dialog has focus.

## The running entry

A user has at most one running entry. Its block is marked as running and grows with the clock. Start a second one and the server answers `409` with the entry that is already running; the CLI and the web UI both say where it is.

A running entry is retimed by stopping it, not by dragging. To backdate the start, edit it after stopping or change it in the [CLI](./cli.md) with `tstamps edit`.

## Overlaps

Entries on different projects may overlap and both count: parallel work is real, and the report sums both. Two entries on the same project may not overlap in time. While you drag, a clashing draft turns red and is not saved on release; the edit panel names the overlapping entry and keeps **Save** disabled until the time is free. The API enforces the same rule with `409 entry-overlap`.

## Timezones

The browser sends its IANA timezone with each new entry, and the entry keeps it. That zone decides which local day and week the hours count in, even if you later change your profile timezone. A day worked in Sydney counts as Sydney's Monday; a lead in Berlin reading the report sees the same hours under the same day.

Bare times typed in the CLI are read in the machine's zone, not the profile zone, so a traveller types the time they are living in.

## Deleting

`Delete` removes an entry from the timeline and from every report total, but the row stays in the database with `deleted_at`. An admin can restore it under **Admin → Entries**. Nothing is hard-deleted.
