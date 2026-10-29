import { describe, expect, it } from 'vitest'
import {
  bucketRange,
  chartSeries,
  csvHref,
  driftOf,
  pivot,
  planByProject,
  presetRange,
  summarize,
  weekStartOf,
} from './reports'
import type { PlanCell, ReportRow } from './reports'

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

describe('plan drift', () => {
  // planCell builds one planned person, project and ISO week.
  function planCell(weekStart: string, userID: number, projectID: number, hours: number): PlanCell {
    return {
      user_id: userID,
      project_id: projectID,
      project: projectID === 1 ? 'Site' : 'App',
      archived: false,
      week_start: weekStart,
      hours,
    }
  }

  it('spreads a week evenly over its days and clamps it to a partial column', () => {
    const [october] = bucketRange('2026-10-01', '2026-11-01', 'month')
    const plan = planByProject([planCell('2026-09-28', 1, 1, 14)], [october])

    // September 28 to October 4 is four October days out of seven.
    expect(plan.get(1)?.seconds['2026-10']).toBeCloseTo((14 * 3600 * 4) / 7)
    expect(plan.get(1)?.total).toBeCloseTo((14 * 3600 * 4) / 7)
  })

  it('keeps the first and last week column inside the range', () => {
    const weeks = bucketRange('2026-10-01', '2026-11-01', 'week')

    expect(weeks[0]).toEqual({ key: '2026-W40', label: 'W40', start: '2026-10-01', end: '2026-10-05' })
    expect(weeks[4].start).toBe('2026-10-26')
    expect(weeks[4].end).toBe('2026-11-01')
  })

  it('starts the plan range at the Monday of the report range', () => {
    expect(weekStartOf('2026-10-01')).toBe('2026-09-28')
    expect(weekStartOf('2026-10-19')).toBe('2026-10-19')
  })

  it('sums people into the project they are planned on', () => {
    const buckets = bucketRange('2026-10-19', '2026-10-26', 'week')
    const plan = planByProject(
      [planCell('2026-10-19', 1, 1, 10), planCell('2026-10-19', 2, 1, 6)],
      buckets,
    )

    expect(plan.get(1)?.total).toBe(16 * 3600)
    expect(plan.get(1)?.people.get(1)?.total).toBe(10 * 3600)
    expect(plan.get(1)?.people.get(2)?.total).toBe(6 * 3600)
  })

  it('keeps a planned project with no logged hours, after the logged ones', () => {
    const buckets = bucketRange('2026-10-19', '2026-10-26', 'week')
    const plan = planByProject([planCell('2026-10-19', 1, 2, 10)], buckets)

    const projects = pivot([row('2026-W43', 1, 1, 3600)], plan)

    expect(projects.map((project) => project.project_id)).toEqual([1, 2])
    expect(projects[1].total).toBe(0)
    expect(projects[1].planned['2026-W43']).toBe(10 * 3600)
    expect(projects[1].plannedTotal).toBe(10 * 3600)
  })

  it('attaches a person plan to an existing person row', () => {
    const buckets = bucketRange('2026-10-19', '2026-10-26', 'week')
    const plan = planByProject([planCell('2026-10-19', 1, 1, 10)], buckets)

    const [site] = pivot([row('2026-W43', 1, 1, 7200)], plan)

    expect(site.people[0].planned['2026-W43']).toBe(10 * 3600)
    expect(site.people[0].plannedTotal).toBe(10 * 3600)
  })

  it('renders logged hours with the signed difference to plan', () => {
    expect(driftOf(7200, undefined)).toEqual({ value: '2h 0m', drift: null })
    expect(driftOf(7200, 3600)).toEqual({ value: '2h 0m', drift: '+1h 0m' })
    expect(driftOf(3600, 7200)).toEqual({ value: '1h 0m', drift: '-1h 0m' })
    expect(driftOf(3600, 3600)).toEqual({ value: '1h 0m', drift: 'on plan' })
    expect(driftOf(undefined, 3600)).toEqual({ value: '0m', drift: '-1h 0m' })
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

describe('chartSeries', () => {
  it('stacks hours per project over every bucket, in the table order', () => {
    const projects = pivot([row('2026-10-19', 1, 1, 3600), row('2026-10-20', 1, 2, 5400)])
    const buckets = bucketRange('2026-10-19', '2026-10-22', 'day')

    const series = chartSeries(projects, buckets, (id) => (id === 1 ? '#111' : '#222'))

    expect(series.labels).toEqual(['Mon 19', 'Tue 20', 'Wed 21'])
    expect(series.datasets).toEqual([
      { label: 'App', data: [0, 1.5, 0], backgroundColor: '#222', borderRadius: 5 },
      { label: 'Site', data: [1, 0, 0], backgroundColor: '#111', borderRadius: 5 },
    ])
  })

  it('draws no datasets when there are no projects', () => {
    expect(chartSeries([], bucketRange('2026-10-19', '2026-10-20', 'day'), () => '#000').datasets).toEqual([])
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
    expect(csvHref({ from: '2026-10-19', to: '2026-10-26', teamID: '', userID: 7 })).toBe(
      '/api/v1/reports.csv?from=2026-10-19&to=2026-10-26&user_id=7',
    )
    expect(csvHref({ from: '2026-10-19', to: '2026-10-26', teamID: '' }, '/w/example.com')).toBe(
      '/w/example.com/api/v1/reports.csv?from=2026-10-19&to=2026-10-26',
    )
  })
})
