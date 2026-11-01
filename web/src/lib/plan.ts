import type { components } from './api/schema'

// Calendar arithmetic for the plan board. All dates are plain YYYY-MM or
// YYYY-MM-DD strings computed in UTC, so a browser west of Greenwich cannot see
// last month's first day as two months ago.
export type PlanPeriod = 'week' | 'month' | 'quarter'

export type Project = components['schemas']['ProjectResponse']
export type Person = components['schemas']['PlanPersonResponse']
export type PlanSummary = components['schemas']['PlanSummaryResponse']
export type PlanSkip = components['schemas']['PlanSummarySkip']

// skipReasons is how the server's reason codes read to a user.
const skipReasons: Record<string, string> = {
  occupied: 'already planned',
  frozen: 'week has ended',
  inactive: 'person is deactivated',
  archived: 'project is archived',
  'other-team': 'project is outside their teams',
  forbidden: 'not yours to plan',
  hours: 'hours outside 0–168',
}

// skippedDetails names one entry per week and reason, so a week that was
// skipped for several people is not repeated.
function skippedDetails(skips: PlanSkip[]): string {
  const groups = new Map<string, { week: string; reason: string; count: number }>()
  for (const skip of skips) {
    const key = `${skip.week_start} ${skip.reason}`
    const group = groups.get(key)
    if (group) group.count += 1
    else groups.set(key, { week: skip.week_start, reason: skip.reason, count: 1 })
  }
  return [...groups.values()]
    .map(({ week, reason, count }) => {
      const clause = skipReasons[reason] ?? reason
      return count > 1 ? `${weekLabel(week)} ${clause} (${count} cells)` : `${weekLabel(week)} ${clause}`
    })
    .join(', ')
}

// summaryNotice says what a bulk action did and why it left the rest alone, so
// a run that wrote nothing explains itself instead of guessing.
export function summaryNotice(verb: string, sum: PlanSummary | undefined): string {
  const filled = sum?.filled ?? 0
  const skipped = sum?.skipped ?? 0
  const message = `${verb} ${filled} ${filled === 1 ? 'cell' : 'cells'}, skipped ${skipped}`
  const detail = skippedDetails(sum?.skipped_detail ?? [])
  return detail ? `${message} — ${detail}.` : `${message}.`
}

// assignableProjects is the plan editor's picker: a cell may only name a project
// of the planned person's teams (ADR-0023), and an archived project can never
// take new plan hours, so the server refuses every cell written against it.
export function assignableProjects(projects: Project[], person: Person | undefined): Project[] {
  const teams = person?.team_ids ?? []
  return projects.filter((project) => project.archived_at == null && teams.includes(project.team_id))
}

export function nextMonth(day: string): string {
  const [year, month] = day.split('-').map(Number)
  return new Date(Date.UTC(year, month, 1)).toISOString().slice(0, 7)
}

export function previousMonth(value: string): string {
  const [year, month] = value.split('-').map(Number)
  return new Date(Date.UTC(year, month - 2, 1)).toISOString().slice(0, 7)
}

function iso(date: Date): string {
  return date.toISOString().slice(0, 10)
}

// planRange turns one day into the half-open range of weeks the board shows.
// The week starts on Monday, the month on its first day, the quarter on the
// first day of its first month.
export function planRange(period: PlanPeriod, day: string): { from: string; to: string } {
  const date = new Date(`${day}T00:00:00Z`)
  if (period === 'week') {
    const from = new Date(date.getTime() - ((date.getUTCDay() + 6) % 7) * 86_400_000)
    return { from: iso(from), to: iso(new Date(from.getTime() + 7 * 86_400_000)) }
  }
  const months = period === 'quarter' ? 3 : 1
  const year = date.getUTCFullYear()
  const month = Math.floor(date.getUTCMonth() / months) * months
  return { from: iso(new Date(Date.UTC(year, month, 1))), to: iso(new Date(Date.UTC(year, month + months, 1))) }
}

export function periodLabel(period: PlanPeriod, day: string): string {
  const { from } = planRange(period, day)
  if (period === 'week') return `Week of ${weekLabel(from)} ${from.slice(0, 4)}`
  if (period === 'month') return monthLabel(from.slice(0, 7))
  const date = new Date(`${from}T00:00:00Z`)
  return `Q${Math.floor(date.getUTCMonth() / 3) + 1} ${date.getUTCFullYear()}`
}

export function monthRange(value: string): { from: string; to: string } {
  return planRange('month', `${value}-01`)
}

// weekEnded mirrors plan.Ended: the week that starts on the Monday weekStart is
// history from the following Monday's midnight in the planned person's own
// timezone, so the current week stays editable. A timezone Intl cannot read is
// UTC, the same fallback the server makes.
export function weekEnded(weekStart: string, timezone: string, now: Date): boolean {
  const end = iso(new Date(Date.parse(`${weekStart}T00:00:00Z`) + 7 * 86_400_000))
  return zoneDay(now, timezone) >= end
}

// zoneDay is now's calendar date where the planned person lives. Its date turns
// over at their midnight, which is exactly where a week ends.
function zoneDay(now: Date, timezone: string): string {
  const parts = zoneFormatter(timezone).formatToParts(now)
  const part = (type: string) => parts.find((entry) => entry.type === type)?.value ?? '00'
  return `${part('year')}-${part('month')}-${part('day')}`
}

const zoneFormatters = new Map<string, Intl.DateTimeFormat>()

function zoneFormatter(timezone: string): Intl.DateTimeFormat {
  const key = timezone === '' ? 'UTC' : timezone
  const cached = zoneFormatters.get(key)
  if (cached) return cached
  const options: Intl.DateTimeFormatOptions = { year: 'numeric', month: '2-digit', day: '2-digit' }
  let format: Intl.DateTimeFormat
  try {
    format = new Intl.DateTimeFormat('en-US', { ...options, timeZone: key })
  } catch {
    format = new Intl.DateTimeFormat('en-US', { ...options, timeZone: 'UTC' })
  }
  zoneFormatters.set(key, format)
  return format
}

export function monthLabel(value: string): string {
  return new Intl.DateTimeFormat('en-GB', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
    new Date(`${value}-01T00:00:00Z`),
  )
}

export function weekLabel(day: string): string {
  return new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(
    new Date(`${day}T00:00:00Z`),
  )
}
