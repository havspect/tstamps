import { currentWeek, todayIn } from './format'

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
export type Bucket = { key: string; label: string }

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
  if (by === 'month') {
    const last = new Date(end.getTime() - DAY_MS)
    let y = start.getUTCFullYear()
    let m = start.getUTCMonth()
    while (y < last.getUTCFullYear() || (y === last.getUTCFullYear() && m <= last.getUTCMonth())) {
      buckets.push({ key: `${y}-${String(m + 1).padStart(2, '0')}`, label: MONTH_NAMES[m] })
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
  const last = new Date(end.getTime() - DAY_MS)
  const monthSpans = start.getUTCMonth() !== last.getUTCMonth() || start.getUTCFullYear() !== last.getUTCFullYear()
  if (by === 'week') cursor.setUTCDate(cursor.getUTCDate() - ((cursor.getUTCDay() + 6) % 7))
  for (; cursor < end; cursor.setUTCDate(cursor.getUTCDate() + step)) {
    if (by === 'day') {
      const day = `${DAY_NAMES[(cursor.getUTCDay() + 6) % 7]} ${cursor.getUTCDate()}`
      buckets.push({
        key: cursor.toISOString().slice(0, 10),
        label: monthSpans ? `${day} ${MONTH_NAMES[cursor.getUTCMonth()]}` : day,
      })
    } else {
      const week = isoWeekKey(cursor)
      buckets.push({ key: week, label: week.slice(5) })
    }
  }
  return buckets
}

export type ReportPerson = { user_id: number; user: string; cells: Record<string, number>; total: number }

export type ReportProject = {
  project_id: number
  project: string
  archived: boolean
  cells: Record<string, number>
  total: number
  people: ReportPerson[]
}

// pivot turns flat (bucket, user, project) rows into one row per project, with
// its people inside it. Projects and people are ordered by hours, biggest
// first; archived projects always sit last.
export function pivot(rows: ReportRow[]): ReportProject[] {
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
      person = { user_id: row.user_id, user: row.user, cells: {}, total: 0 }
      inProject.set(row.user_id, person)
      project.people.push(person)
    }
    add(person.cells, row.bucket, row.seconds)
    person.total += row.seconds
  }

  const out = [...projects.values()]
  for (const project of out) project.people.sort((a, b) => b.total - a.total || a.user.localeCompare(b.user))
  out.sort((a, b) => Number(a.archived) - Number(b.archived) || b.total - a.total || a.project.localeCompare(b.project))
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

export type CsvFilters = { from: string; to: string; teamID: string }

// csvHref is the download URL for the filters the table was loaded with; an
// empty id means "all", and the API ignores the bucket because CSV is per day.
export function csvHref({ from, to, teamID }: CsvFilters, base = ''): string {
  const query = new URLSearchParams({ from, to })
  if (teamID) query.set('team_id', teamID)
  return `${base}/api/v1/reports.csv?${query}`
}
