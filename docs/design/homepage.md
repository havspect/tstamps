# Homepage — design document

Status: agreed direction, implemented at `/` for signed-out visitors.
Owner: web. Date: 2026-10-24.

## 1. What we are selling

Tstamps is self-hosted team time tracking for one company. The product's
distinctive idea is small enough to say in one sentence:

> A week of hours is a few drags, not a Friday form.

The homepage has to make that idea visible before asking for a login. It is
**not** a dashboard and it is not a second product: it is the front door of the
same single binary, shown only when nobody is signed in.

### Goals

1. A visitor understands what Tstamps is, who it is for and what it is not, in
   under 20 seconds.
2. The timeline is shown, not described. The hero contains the real interface.
3. A person with a workspace knows exactly where to sign in; a person without
   one knows how to run it.
4. It loads with the same static SPA: no external fonts, trackers, CDNs or
   server-side rendering.
5. It feels like Tstamps: paper-warm, quiet, precise, with project colour as the
   only decoration.

### Non-goals

- No pricing table, no “Book a demo”, no invented customer logos or quotes.
- No sign-up flow. A workspace is bootstrapped by its admin (`admin create`).
- No marketing claims the spec cannot keep (approvals, invoicing, SSO, mobile).
- No separate landing app or static-site build.

## 2. Evidence: the product as it is today

These screenshots were captured from a demo workspace running the current
server, with four people, two teams and a week of entries. They are the visual
source of truth; the landing page reuses crops of the real UI rather than
drawing a prettier fake.

### Timeline — the core screen

![Timeline at 1440×900](screenshots/timeline.png)

What works: the lane-per-project layout is immediately readable; project colour
does the wayfinding; the boxes are the interface, not decoration. The drag
affordances and keyboard hints are present but quiet.

What the homepage must preserve: the block shapes, the 6px radius, the green
`#2f5d50`, the paper background, the mono-ish timestamps, and the sense of a
workday laid out vertically.

### Reports — the reason a lead opens it

![Reports at 1440×900](screenshots/reports.png)

The table is confident: one row per project, one column per day, total on the
right, grand total at the bottom. It is a record, not a dashboard. The landing
page should use this as evidence for “read the week without asking for a form”.

### Admin — the workspace is yours

![Admin users at 1440×900](screenshots/admin-users.png)

Dense, utilitarian, no settings maze. Kept out of the homepage except as a
supporting claim (“users, teams and projects live in the same binary”).

### Sign in — the only door

![Sign in at 1440×900](screenshots/login.png)

The homepage must hand off to this screen without a second identity system.

### Narrow viewport — known constraint

![Timeline at 390×844](screenshots/timeline-mobile.png)

The app shell is desktop-first: the header and the report table overflow at
390px. This is out of scope for the homepage change, but the homepage itself
must be fully responsive and must not pretend the app is a mobile product.

## 3. Visual direction

**Concept: the paper ledger with a live timeline.** Warm paper, fine rules,
monospace time labels, one deep green and the four project colours. The page is
laid out like a well-kept record: generous margins, short lines, no cards inside
cards. The only large image is the product itself.

### Tokens (reuse the app, add nothing global)

| Role | Value | Use |
| --- | --- | --- |
| Paper | `#f6f6f3` | page background |
| Surface | `#ffffff` | screenshot frames, code block |
| Ink | `#1d1d1a` | headings and body |
| Muted | `#6b6b63` | supporting copy, labels |
| Rule | `#d9d9d1` | borders, section rules |
| Accent | `#2f5d50` | CTAs, links, highlights |
| Accent deep | `#24483e` | hover |
| Night | `#1c352d` | self-host section |
| Project colours | `#a4602c`, `#3d6f8e`, `#7a5a8f`, `#8e6d3d` | swatches, small accents |

Type: the app's `system-ui` stack throughout. Display sizes use weight 650 and
tight tracking (`-0.04em`), not a new webfont. Labels use `ui-monospace` at
0.75rem, uppercase, 0.08em tracking. This is the timestamp language of the
product.

Shape: 6px radius on controls and screenshots, 10px on the hero frame. One
shadow only, under the hero image (`0 40px 80px -48px rgb(29 29 26 / 45%)`).

Motion: a single 500ms rise on the hero copy and image, and a slow pulse on the
“recording” dot. Disabled under `prefers-reduced-motion`.

### Image treatment

- Hero: a 2× WebP crop of `.lanes-scroll`, 2200×1402, 36 KB. It is the real
  timeline, no invented UI.
- Reports: a 2× WebP of the reports `<main>`, 2048×1138, 58 KB, lazy loaded.
- Screenshots sit in a white `padding: 6px` frame with a hairline border, like a
  print in a mat. No tilt, no faux browser chrome, no gradient overlays.
- A small hand-positioned tag (“drag to log”) is the only annotation.

## 4. Page anatomy

```
┌──────────────────────────────────────────────────────────────┐
│ skip link                                                     │
│ Tstamps   Timeline  Reports  CLI  Self-host        [Sign in]  │
├──────────────────────────────────────────────────────────────┤
│ SELF-HOSTED TEAM TIME TRACKING                               │
│ Your week,                                                    │
│ in a few drags.            ┌───────────────────────────┐      │
│                            │  real timeline crop       │      │
│ Three sentences…           │  [drag to log]            │      │
│                            └───────────────────────────┘      │
│ [Sign in] [See the timeline]                                  │
│ AGPL-3.0 · No per-seat pricing · One VM                       │
├──────────────────────────────────────────────────────────────┤
│ 01 THE LOOP                                                   │
│ Drag, don't fill in.  Read the week.  Log from anywhere.      │
│ three short columns, each with a small line visual            │
├──────────────────────────────────────────────────────────────┤
│ 02 REPORTS                       ┌─────────────────────────┐  │
│ The report is the meeting.       │ reports screenshot      │  │
│ By person, by day, CSV.          └─────────────────────────┘  │
├──────────────────────────────────────────────────────────────┤
│ 03 CLI + MCP          ┌────────────────────────────────────┐  │
│ Works where you do.   │ $ tstamps start Timeline           │  │
│                      └────────────────────────────────────┘  │
├──────────────────────────────────────────────────────────────┤
│ NIGHT SECTION: Yours to run.                                  │
│ One binary · one SQLite file · backup is a file copy          │
│ [install commands]                                            │
├──────────────────────────────────────────────────────────────┤
│ Deliberately small.                                           │
│ Two columns: what it does / what it leaves out.               │
├──────────────────────────────────────────────────────────────┤
│ Open your workspace.                            [Sign in]     │
│ AGPL-3.0 · GitHub · Spec · Glossary · Decisions               │
└──────────────────────────────────────────────────────────────┘
```

### Copy (final)

**Hero**

- Eyebrow: `SELF-HOSTED TEAM TIME TRACKING`
- H1: `Your week, in a few drags.` — “few drags” sits in an accent block, shaped
  like a timeline entry.
- Body: “Tstamps is a team time tracker built around a timeline. Drag on a lane
  to log an entry, drag its edges to retime it, and let the report do the
  remembering. One Go binary, one SQLite file, yours to run.”
- Primary: `Sign in`
- Secondary: `See the timeline`
- Trust line: `AGPL-3.0` · `No per-seat pricing` · `One VM`

**The loop**

- Heading: `Three moves, no timesheet.`
- 01 `Drag, don't fill in.` — “Drag on a lane to log; drag an edge to retime;
  press space to start and stop. Overlaps are shown, not rejected.”
- 02 `Read the week.` — “Projects as rows, days as columns, biggest first. Open a
  project for one line per person, then take the CSV.”
- 03 `Log from anywhere.` — “A one-line CLI and an MCP server share the same API,
  so a terminal or an agent can start, stop and correct entries.”

**Reports**

- Label: `02 — REPORTS`
- Heading: `The report is the meeting.`
- Body: “Leads read every hour booked on their team's projects, by person and by
  day, week or month. There is no form to chase and no separate analytics tool
  to reconcile. Export the same rows as CSV.”
- Detail list: “Entry timezone decides the day”, “Archived projects keep their
  hours”, “Read-only for members”.

**CLI + MCP**

- Label: `03 — CLI & AGENTS`
- Heading: `Works where you do.`
- Body: “`tstamps start` is one line in a terminal. `tstamps mcp` lets an agent
  start, stop and correct entries with the permissions of the token you gave it.
  Both talk to the same HTTP API as the timeline.”
- Terminal card: install, login, start, stop, report.

**Self-host**

- Heading: `Yours to run.`
- Body: “One `tstamps-server` process on one VM, behind Caddy on the same host.
  The UI is embedded in the binary; the data is one SQLite file. Backups use
  `VACUUM INTO` and are safe while the server is running.”
- Facts: `One binary`, `One SQLite file`, `No containers`.
- Commands: the release download and `tstamps-server serve`.

**Deliberately small**

- Heading: `Deliberately small.`
- Intro: “Tstamps is not trying to run your whole company. If you need these,
  choose something else — and that is the point.”
- Left (it does): timeline logging · team-lead reports · CSV · CLI + MCP ·
  soft delete and restore · entry timezones · one workspace.
- Right (it does not): approvals or leave · invoicing and rates · SSO or audit
  log · mobile app · realtime co-editing · several companies per install.
- Footer of section: “What it does and does not do is in the spec.”

**Close**

- Heading: `Open your workspace.`
- Body: “If your server is already running, the timeline is one sign-in away.”
- Primary: `Sign in`. Secondary: `Read the specification`.

**Footer**

`Tstamps` · AGPL-3.0-only · GitHub · Specification · Glossary · Decisions ·
Built with Go and Svelte.

## 5. Responsive behaviour

- `≥ 60rem`: two-column hero (`1.05fr / 0.95fr`), reports split 1:1.2.
- `< 60rem`: hero stacks, image after the CTAs; nav links collapse to brand +
  sign in; feature rows stack; the night section keeps its inset.
- `< 40rem`: H1 `clamp(2.5rem, 12vw, 3.5rem)`; code block scrolls horizontally;
  all touch targets at least 44px.

## 6. Accessibility and performance

- Landmark structure: one `header`, one `main`, one `footer`, labelled nav.
- The hero image has descriptive alt text; the reports image is lazy and has its
  own dimensions to avoid layout shift.
- Colour contrast: ink on paper 13.9:1; muted on paper 5.2:1; white on accent
  7.4:1. The accent block behind “few drags” uses white text.
- Focus: the app's 2px accent ring, with `scroll-margin-top` on targets.
- Motion respects `prefers-reduced-motion`.
- Static HTML/CSS after hydration: no API call, no new dependency, ~100 KB of
  images (WebP), both served from the embedded SPA. No external requests.
- Signed-in visitors are redirected to `/timeline`; signed-out visitors on any
  other route still land on `/login`.

## 7. Implementation notes

- New route `/` → `web/src/pages/Home.svelte`, styles scoped to that component.
- `App.svelte` renders `Home` without the app header/main; the auth guard lets
  anonymous users stay on `/` and sends signed-in users to `/timeline`.
- Assets in `web/public/screenshots/` (Vite copies them into `web/dist` and
  `go:embed` carries them).
- The page is public: no session, no API, no data.
- E2E: assert the landing page shows the H1 and a sign-in link, and that signing
  in still reaches the timeline.
- Design screenshots live in `docs/design/screenshots/` and are not shipped.

## 8. Acceptance

- [ ] `/` explains the product and the CTA in one screen at 1440×900.
- [ ] No layout overflow at 320, 390, 768, 1024 or 1440 CSS px.
- [ ] Keyboard: skip link, focus visible, every anchor reachable.
- [ ] Lighthouse: no contrast or landmark failures; CLS < 0.05; images < 150 KB.
- [ ] Anonymous `/` renders with no uncaught JS errors (the expected anonymous
      `/me` 401 is the only failed request).
- [ ] Signed-in `/` redirects to `/timeline`; `/login` still works.

## 9. Result

The implementation, captured after the build:

![The implemented homepage at 1440px](screenshots/home.png)

