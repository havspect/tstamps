// Calendar arithmetic for the plan board. All dates are plain YYYY-MM or
// YYYY-MM-DD strings computed in UTC, so a browser west of Greenwich cannot see
// last month's first day as two months ago.
export type PlanPeriod = 'week' | 'month' | 'quarter'

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
