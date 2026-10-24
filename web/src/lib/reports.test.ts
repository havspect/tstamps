import { describe, expect, it } from 'vitest'
import { bucketRange, csvHref, pivot, presetRange, summarize } from './reports'
import type { ReportRow } from './reports'

// row builds a report row; user 1 is Ada, user 2 is Grace.
function row(bucket: string, userID: number, projectID: number, seconds: number, archived = false): ReportRow {
  return {
    bucket,
    user_id: userID,
    user: userID === 1 ? 'Ada' : 'Grace',
    project_id: projectID,
    project: projectID === 1 ? 'Site' : 'App',
    archived,
    seconds,
  }
}

describe('bucketRange', () => {
  it('lists every day of the range, empty or not', () => {
    const days = bucketRange('2026-10-19', '2026-10-26', 'day')

    expect(days.map((day) => day.key)).toEqual([
      '2026-10-19',
      '2026-10-20',
      '2026-10-21',
      '2026-10-22',
      '2026-10-23',
      '2026-10-24',
      '2026-10-25',
    ])
    expect(days[0].label).toBe('Mon 19')
    expect(days[6].label).toBe('Sun 25')
  })

  it('starts weeks on the Monday on or before the range', () => {
    const weeks = bucketRange('2026-10-01', '2026-11-01', 'week')

    expect(weeks.map((week) => week.key)).toEqual([
      '2026-W40',
      '2026-W41',
      '2026-W42',
      '2026-W43',
      '2026-W44',
    ])
    expect(weeks[0].label).toBe('W40')
  })

  it('uses the ISO week year at the turn of the year', () => {
    expect(bucketRange('2026-12-28', '2027-01-04', 'week').map((week) => week.key)).toEqual(['2026-W53'])
  })

  it('lists calendar months, last one included', () => {
    const months = bucketRange('2026-10-19', '2027-01-01', 'month')

    expect(months.map((month) => month.key)).toEqual(['2026-10', '2026-11', '2026-12'])
    expect(months.map((month) => month.label)).toEqual(['Oct', 'Nov', 'Dec'])
  })

  it('returns nothing for an empty or backwards range', () => {
    expect(bucketRange('2026-10-19', '2026-10-19', 'day')).toEqual([])
    expect(bucketRange('2026-10-26', '2026-10-19', 'day')).toEqual([])
  })

  it('adds the month to day labels only when the range crosses one', () => {
    const days = bucketRange('2026-10-26', '2026-11-03', 'day')

    expect(days[0].label).toBe('Mon 26 Oct')
    expect(days.find((day) => day.key === '2026-11-01')?.label).toBe('Sun 1 Nov')
  })
})

describe('pivot', () => {
  it('sums a project per bucket and in total', () => {
    const [site] = pivot([
      row('2026-10-19', 1, 1, 3600),
      row('2026-10-19', 2, 1, 1800),
      row('2026-10-20', 1, 1, 600),
    ])

    expect(site.cells).toEqual({ '2026-10-19': 5400, '2026-10-20': 600 })
    expect(site.total).toBe(6000)
  })

  it('keeps the people inside each project, biggest first', () => {
    const [site] = pivot([row('2026-10-19', 1, 1, 600), row('2026-10-19', 2, 1, 3600)])

    expect(site.people.map((person) => person.user)).toEqual(['Grace', 'Ada'])
    expect(site.people[0].cells).toEqual({ '2026-10-19': 3600 })
  })

  it('orders projects by hours with archived ones last', () => {
    const projects = pivot([
      row('2026-10-19', 1, 1, 600),
      row('2026-10-19', 1, 2, 3600),
      row('2026-10-19', 1, 3, 900, true),
    ])

    expect(projects.map((project) => project.project_id)).toEqual([2, 1, 3])
  })

  it('returns nothing for no rows', () => {
    expect(pivot([])).toEqual([])
  })
})

describe('summarize', () => {
  it('totals hours, counts people once across projects and averages per head', () => {
    const summary = summarize(
      pivot([
        row('2026-10-19', 1, 1, 3600),
        row('2026-10-20', 2, 1, 1800),
        row('2026-10-20', 1, 2, 600),
      ]),
    )

    expect(summary.total).toBe(6000)
    expect(summary.people).toBe(2)
    expect(summary.perPerson).toBe(3000)
    expect(summary.busiest?.project).toBe('Site')
    expect(summary.busiest?.share).toBeCloseTo(0.9)
  })

  it('lets an archived project be the busiest, as the table counts it', () => {
    const summary = summarize(pivot([row('2026-10-19', 1, 1, 600), row('2026-10-19', 1, 2, 3600, true)]))

    expect(summary.busiest).toEqual({ project: 'App', archived: true, share: 3600 / 4200 })
  })

  it('has no busiest project and no average when nothing was logged', () => {
    expect(summarize([])).toEqual({ total: 0, people: 0, perPerson: 0, busiest: null })
  })
})

describe('presetRange', () => {
  // Friday 23 October 2026, midday UTC.
  const now = new Date('2026-10-23T12:00:00Z')

  it('reads a week as days', () => {
    expect(presetRange('this-week', 'UTC', now)).toEqual({ from: '2026-10-19', to: '2026-10-26', by: 'day' })
    expect(presetRange('last-week', 'UTC', now)).toEqual({ from: '2026-10-12', to: '2026-10-19', by: 'day' })
  })

  it('reads a month as weeks', () => {
    expect(presetRange('this-month', 'UTC', now)).toEqual({ from: '2026-10-01', to: '2026-11-01', by: 'week' })
    expect(presetRange('last-month', 'UTC', now)).toEqual({ from: '2026-09-01', to: '2026-10-01', by: 'week' })
  })

  it('reads a year as months', () => {
    expect(presetRange('this-year', 'UTC', now)).toEqual({ from: '2026-01-01', to: '2027-01-01', by: 'month' })
  })

  it('uses the person timezone for the month', () => {
    // 31 October 23:30 UTC is already 1 November in Berlin.
    const range = presetRange('this-month', 'Europe/Berlin', new Date('2026-10-31T23:30:00Z'))
    expect(range.from).toBe('2026-11-01')
  })
})

describe('csvHref', () => {
  it('always sends the range and only the chosen team', () => {
    expect(csvHref({ from: '2026-10-19', to: '2026-10-26', teamID: '' })).toBe(
      '/api/v1/reports.csv?from=2026-10-19&to=2026-10-26',
    )

    const query = new URLSearchParams(csvHref({ from: '2026-10-19', to: '2026-10-26', teamID: '2' }).split('?')[1])
    expect(query.get('team_id')).toBe('2')
    expect(csvHref({ from: '2026-10-19', to: '2026-10-26', teamID: '' }, '/w/example.com')).toBe(
      '/w/example.com/api/v1/reports.csv?from=2026-10-19&to=2026-10-26',
    )
  })
})
