import { currentWeek, formatDuration, todayIn } from './format'

// Report rows as the API returns them, narrowed to the fields the page reads.
export type ReportRow = {
  bucket: string
  user_id: number
  user: string
  project_id: number
  project: string
  archived: boolean
  seconds: number
}

export type BucketBy = 'day' | 'week' | 'month'

// Bucket is one time column: a person-local day, ISO week or calendar month.
// start and end are the days it covers inside the range, end exclusive.
export type Bucket = { key: string; label: string; start: string; end: string }

// PlanCell is one planned person, project and ISO week, as the plan API returns it.
export type PlanCell = {
  user_id: number
  project_id: number
  project: string
  archived: boolean
  week_start: string
  hours: number
}

// PlannedProject is a project's plan for the report's columns, in seconds.
export type PlannedProject = {
  project: string
  archived: boolean
  seconds: Record<string, number>
  total: number
  people: Map<number, { seconds: Record<string, number>; total: number }>
}

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const DAY_MS = 86_400_000

// isoWeekKey is the API's ISO week label ("2026-W43") for a UTC-midnight date.
function isoWeekKey(date: Date): string {
  const thursday = new Date(date.getTime())
  thursday.setUTCDate(thursday.getUTCDate() + 4 - (thursday.getUTCDay() || 7))
  const year = thursday.getUTCFullYear()
  const week = Math.ceil(((thursday.getTime() - Date.UTC(year, 0, 1)) / DAY_MS + 1) / 7)
  return `${year}-W${String(week).padStart(2, '0')}`
}

// bucketRange lists every column of the range, including empty ones: the API
// only returns buckets that hold hours. from is inclusive, to exclusive.
export function bucketRange(from: string, to: string, by: BucketBy): Bucket[] {
  const start = new Date(`${from}T00:00:00Z`)
  const end = new Date(`${to}T00:00:00Z`)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || end <= start) return []

  const buckets: Bucket[] = []
  const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10)
  // Clamp each column to the range: hours and plans only exist inside it.
  const add = (key: string, label: string, fromMs: number, toMs: number) => {
    const s = Math.max(fromMs, start.getTime())
    const e = Math.min(toMs, end.getTime())
    if (e > s) buckets.push({ key, label, start: iso(s), end: iso(e) })
  }
  const last = new Date(end.getTime() - DAY_MS)

  if (by === 'month') {
    let y = start.getUTCFullYear()
    let m = start.getUTCMonth()
    while (y < last.getUTCFullYear() || (y === last.getUTCFullYear() && m <= last.getUTCMonth())) {
      add(`${y}-${String(m + 1).padStart(2, '0')}`, MONTH_NAMES[m], Date.UTC(y, m, 1), Date.UTC(y, m + 1, 1))
      if (++m === 12) {
        m = 0
        y++
      }
    }
    return buckets
  }

  const step = by === 'day' ? 1 : 7
  const cursor = new Date(start)
  // A day label needs its month only when the range crosses one; otherwise
  // "Mon 19" is enough and the header stays short.
  const monthSpans = start.getUTCMonth() !== last.getUTCMonth() || start.getUTCFullYear() !== last.getUTCFullYear()
  if (by === 'week') cursor.setUTCDate(cursor.getUTCDate() - ((cursor.getUTCDay() + 6) % 7))
  for (; cursor < end; cursor.setUTCDate(cursor.getUTCDate() + step)) {
    if (by === 'day') {
      const day = `${DAY_NAMES[(cursor.getUTCDay() + 6) % 7]} ${cursor.getUTCDate()}`
      add(
        cursor.toISOString().slice(0, 10),
        monthSpans ? `${day} ${MONTH_NAMES[cursor.getUTCMonth()]}` : day,
        cursor.getTime(),
        cursor.getTime() + DAY_MS,
      )
    } else {
      const week = isoWeekKey(cursor)
      add(week, week.slice(5), cursor.getTime(), cursor.getTime() + 7 * DAY_MS)
    }
  }
  return buckets
}

// weekStartOf is the Monday on or before a date, the week the plan API must
// start at to cover the range's first column.
export function weekStartOf(date: string): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7))
  return d.toISOString().slice(0, 10)
}

// planByProject spreads each plan cell evenly over its ISO week's days and
// keeps only the days inside each column, so week and month columns agree.
export function planByProject(cells: PlanCell[], buckets: Bucket[]): Map<number, PlannedProject> {
  const out = new Map<number, PlannedProject>()
  for (const cell of cells) {
    const weekStart = Date.parse(`${cell.week_start}T00:00:00Z`)
    if (Number.isNaN(weekStart) || cell.hours <= 0) continue

    let project = out.get(cell.project_id)
    if (!project) {
      project = { project: cell.project, archived: cell.archived, seconds: {}, total: 0, people: new Map() }
      out.set(cell.project_id, project)
    }
    let person = project.people.get(cell.user_id)
    if (!person) {
      person = { seconds: {}, total: 0 }
      project.people.set(cell.user_id, person)
    }

    for (const bucket of buckets) {
      const start = Math.max(weekStart, Date.parse(`${bucket.start}T00:00:00Z`))
      const end = Math.min(weekStart + 7 * DAY_MS, Date.parse(`${bucket.end}T00:00:00Z`))
      if (end <= start) continue
      const seconds = (cell.hours * 3600 * (end - start)) / (7 * DAY_MS)
      project.seconds[bucket.key] = (project.seconds[bucket.key] ?? 0) + seconds
      project.total += seconds
      person.seconds[bucket.key] = (person.seconds[bucket.key] ?? 0) + seconds
      person.total += seconds
    }
  }
  return out
}

// driftOf renders one cell: the logged hours and, when the cell is planned, the
// signed logged-minus-plan difference. "+" is more than planned.
export function driftOf(
  logged: number | undefined,
  planned: number | undefined,
): { value: string; drift: string | null } {
  if (planned === undefined) return { value: logged ? formatDuration(logged) : '', drift: null }
  const minutes = Math.round(((logged ?? 0) - planned) / 60)
  return {
    value: formatDuration(logged ?? 0),
    drift: minutes === 0 ? 'on plan' : `${minutes > 0 ? '+' : '-'}${formatDuration(Math.abs(minutes) * 60)}`,
  }
}

export type ReportPerson = {
  user_id: number
  user: string
  cells: Record<string, number>
  total: number
  planned: Record<string, number>
  plannedTotal: number
}

export type ReportProject = {
  project_id: number
  project: string
  archived: boolean
  cells: Record<string, number>
  total: number
  planned: Record<string, number>
  plannedTotal: number
  people: ReportPerson[]
}

// pivot turns flat (bucket, user, project) rows into one row per project, with
// its people inside it. Projects and people are ordered by hours, biggest
// first; archived projects always sit last. A project that was planned but had
// no hours logged keeps a row, so its shortfall is visible.
export function pivot(rows: ReportRow[], planned: Map<number, PlannedProject> = new Map()): ReportProject[] {
  const projects = new Map<number, ReportProject>()
  const people = new Map<number, Map<number, ReportPerson>>()
  const add = (cells: Record<string, number>, bucket: string, seconds: number) => {
    cells[bucket] = (cells[bucket] ?? 0) + seconds
  }

  for (const row of rows) {
    let project = projects.get(row.project_id)
    if (!project) {
      project = {
        project_id: row.project_id,
        project: row.project,
        archived: row.archived,
        cells: {},
        total: 0,
        planned: {},
        plannedTotal: 0,
        people: [],
      }
      projects.set(row.project_id, project)
      people.set(row.project_id, new Map())
    }
    add(project.cells, row.bucket, row.seconds)
    project.total += row.seconds

    const inProject = people.get(row.project_id)!
    let person = inProject.get(row.user_id)
    if (!person) {
      person = { user_id: row.user_id, user: row.user, cells: {}, total: 0, planned: {}, plannedTotal: 0 }
      inProject.set(row.user_id, person)
      project.people.push(person)
    }
    add(person.cells, row.bucket, row.seconds)
    person.total += row.seconds
  }

  for (const [projectID, plan] of planned) {
    const project = projects.get(projectID)
    if (!project) {
      projects.set(projectID, {
        project_id: projectID,
        project: plan.project,
        archived: plan.archived,
        cells: {},
        total: 0,
        planned: plan.seconds,
        plannedTotal: plan.total,
        people: [],
      })
      continue
    }
    project.planned = plan.seconds
    project.plannedTotal = plan.total
    for (const person of project.people) {
      const personPlan = plan.people.get(person.user_id)
      if (personPlan) {
        person.planned = personPlan.seconds
        person.plannedTotal = personPlan.total
      }
    }
  }

  const out = [...projects.values()]
  for (const project of out) project.people.sort((a, b) => b.total - a.total || a.user.localeCompare(b.user))
  out.sort(
    (a, b) =>
      Number(a.archived) - Number(b.archived) ||
      b.total - a.total ||
      b.plannedTotal - a.plannedTotal ||
      a.project.localeCompare(b.project),
  )
  return out
}

export type ReportSummary = {
  total: number
  people: number
  perPerson: number
  busiest: { project: string; archived: boolean; share: number } | null
}

// summarize reads the same pivot the table shows, so the KPI band and the
// grand total can never disagree. Archived projects count like any other.
export function summarize(projects: ReportProject[]): ReportSummary {
  const people = new Set<number>()
  let total = 0
  let busiest: ReportProject | null = null
  for (const project of projects) {
    total += project.total
    for (const person of project.people) people.add(person.user_id)
    if (!busiest || project.total > busiest.total) busiest = project
  }
  return {
    total,
    people: people.size,
    perPerson: people.size ? Math.round(total / people.size) : 0,
    busiest: busiest && total > 0
      ? { project: busiest.project, archived: busiest.archived, share: busiest.total / total }
      : null,
  }
}

export type ChartSeries = {
  labels: string[]
  datasets: { label: string; data: number[]; backgroundColor: string; borderRadius: number }[]
}

// chartSeries turns the pivoted projects into stacked bars: one bar per bucket,
// one dataset per project, in hours. The order is the table's, so the legend
// and the table rows read the same way.
export function chartSeries(
  projects: ReportProject[],
  buckets: Bucket[],
  color: (projectID: number) => string,
): ChartSeries {
  return {
    labels: buckets.map((bucket) => bucket.label),
    datasets: projects.map((project) => ({
      label: project.project,
      backgroundColor: color(project.project_id),
      borderRadius: 5,
      data: buckets.map((bucket) => (project.cells[bucket.key] ?? 0) / 3600),
    })),
  }
}

export type RangePreset = 'this-week' | 'last-week' | 'this-month' | 'last-month' | 'this-year' | 'custom'

// presetRange gives the dates and the granularity a preset implies: a week
// reads as days, a month as weeks, a year as months.
export function presetRange(
  preset: Exclude<RangePreset, 'custom'>,
  timezone: string,
  now: Date = new Date(),
): { from: string; to: string; by: BucketBy } {
  const iso = (date: Date) => date.toISOString().slice(0, 10)
  const thisWeek = currentWeek(timezone, now)
  if (preset === 'this-week') return { ...thisWeek, by: 'day' }
  if (preset === 'last-week') {
    const from = new Date(`${thisWeek.from}T00:00:00Z`)
    from.setUTCDate(from.getUTCDate() - 7)
    const to = new Date(`${thisWeek.from}T00:00:00Z`)
    return { from: iso(from), to: iso(to), by: 'day' }
  }

  const today = new Date(`${todayIn(timezone, now)}T00:00:00Z`)
  const year = today.getUTCFullYear()
  const month = today.getUTCMonth()
  if (preset === 'this-month') return { from: iso(new Date(Date.UTC(year, month, 1))), to: iso(new Date(Date.UTC(year, month + 1, 1))), by: 'week' }
  if (preset === 'last-month') return { from: iso(new Date(Date.UTC(year, month - 1, 1))), to: iso(new Date(Date.UTC(year, month, 1))), by: 'week' }
  return { from: iso(new Date(Date.UTC(year, 0, 1))), to: iso(new Date(Date.UTC(year + 1, 0, 1))), by: 'month' }
}

export type CsvFilters = { from: string; to: string; teamID: string; userID?: number }

// csvHref is the download URL for the filters the table was loaded with; an
// empty id means "all", and the API ignores the bucket because CSV is per day.
export function csvHref({ from, to, teamID, userID }: CsvFilters, base = ''): string {
  const query = new URLSearchParams({ from, to })
  if (teamID) query.set('team_id', teamID)
  if (userID) query.set('user_id', String(userID))
  return `${base}/api/v1/reports.csv?${query}`
}
