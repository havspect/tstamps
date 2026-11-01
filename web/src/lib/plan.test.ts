import { describe, expect, it } from 'vitest'
import {
  assignableProjects,
  bulkPreview,
  monthRange,
  nextMonth,
  periodLabel,
  planRange,
  previousMonth,
  summaryNotice,
  weekEnded,
  weekLabel,
} from './plan'
import type { Person, PlanSummary, Project } from './plan'

const project = (id: number, archived_at: string | null, team_id = 1): Project => ({
  id,
  team_id,
  name: `Project ${id}`,
  color: '#4f46e5',
  archived_at,
})

const person = (team_ids: number[] | null): Person => ({
  deactivated: false,
  email: 'ana@example.com',
  team_ids,
  teams: null,
  timezone: 'UTC',
  user: 'Ana',
  user_id: 1,
  weekly_hours: 40,
  weeks: [],
})

describe('plan months', () => {
  it('steps around December and January', () => {
    expect(nextMonth('2026-12-15')).toBe('2027-01')
    expect(previousMonth('2027-01')).toBe('2026-12')
  })

  it('turns a month into the half-open range the API takes', () => {
    expect(monthRange('2026-11')).toEqual({ from: '2026-11-01', to: '2026-12-01' })
    expect(monthRange('2026-12')).toEqual({ from: '2026-12-01', to: '2027-01-01' })
  })

  it('labels a week start without drifting a day in UTC', () => {
    expect(weekLabel('2026-11-02')).toBe('2 Nov')
  })
})

describe('plan ranges', () => {
  it('turns any day into its Monday-to-Monday week', () => {
    expect(planRange('week', '2026-11-04')).toEqual({ from: '2026-11-02', to: '2026-11-09' })
    expect(planRange('week', '2026-11-02')).toEqual({ from: '2026-11-02', to: '2026-11-09' })
    expect(planRange('week', '2027-01-01')).toEqual({ from: '2026-12-28', to: '2027-01-04' })
  })

  it('turns any day into its quarter', () => {
    expect(planRange('quarter', '2026-11-04')).toEqual({ from: '2026-10-01', to: '2027-01-01' })
    expect(planRange('quarter', '2026-02-15')).toEqual({ from: '2026-01-01', to: '2026-04-01' })
  })

  it('labels the range', () => {
    expect(periodLabel('week', '2026-11-04')).toBe('Week of 2 Nov 2026')
    expect(periodLabel('month', '2026-11-04')).toBe('November 2026')
    expect(periodLabel('quarter', '2026-11-04')).toBe('Q4 2026')
    expect(periodLabel('quarter', '2026-02-15')).toBe('Q1 2026')
  })
})

describe('bulk summaries', () => {
  const skip = (week_start: string, reason: string) => ({ week_start, reason, user_id: 5, project_id: 10 })

  it('names every skipped week with its reason', () => {
    const sum: PlanSummary = {
      cells: [],
      filled: 2,
      skipped: 2,
      skipped_detail: [skip('2026-12-28', 'occupied'), skip('2027-01-04', 'occupied')],
    }
    expect(summaryNotice('Filled', sum)).toBe(
      'Filled 2 cells, skipped 2 — 28 Dec already planned, 4 Jan already planned.',
    )
  })

  it('says why a run that wrote nothing was skipped, even for other reasons', () => {
    const sum: PlanSummary = {
      cells: [],
      filled: 0,
      skipped: 2,
      skipped_detail: [skip('2027-01-04', 'frozen'), skip('2027-01-11', 'archived')],
    }
    expect(summaryNotice('Seeded', sum)).toBe(
      'Seeded 0 cells, skipped 2 — 4 Jan week has ended, 11 Jan project is archived.',
    )
  })

  it('states one week once when several cells were skipped there', () => {
    const sum: PlanSummary = {
      cells: [],
      filled: 0,
      skipped: 3,
      skipped_detail: [
        skip('2027-01-04', 'other-team'),
        skip('2027-01-04', 'other-team'),
        skip('2027-01-04', 'other-team'),
      ],
    }
    expect(summaryNotice('Seeded', sum)).toBe(
      'Seeded 0 cells, skipped 3 — 4 Jan project is outside their teams (3 cells).',
    )
  })

  it('keeps the counters when the server reported no reasons', () => {
    expect(summaryNotice('Filled', { cells: [], filled: 0, skipped: 8, skipped_detail: null })).toBe(
      'Filled 0 cells, skipped 8.',
    )
    expect(summaryNotice('Filled', undefined)).toBe('Filled 0 cells, skipped 0.')
  })

  it('says one cell, not one cells', () => {
    expect(summaryNotice('Filled', { cells: [], filled: 1, skipped: 0, skipped_detail: [] })).toBe(
      'Filled 1 cell, skipped 0.',
    )
  })
})

describe('bulk previews', () => {
  const filled = (week_start: string) => ({ week_start, user_id: 5, project_id: 10, hours: 8 })
  const skip = (week_start: string, reason: string) => ({ week_start, reason, user_id: 5, project_id: 10 })

  it('names the weeks it would fill and why the rest would not', () => {
    const sum: PlanSummary = {
      cells: [filled('2026-12-28'), filled('2027-01-04')],
      filled: 2,
      skipped: 1,
      skipped_detail: [skip('2027-01-11', 'occupied')],
    }
    expect(bulkPreview('copy', sum)).toEqual({
      notice: 'Would copy 2 cells into 28 Dec, 4 Jan, skipping 1 — 11 Jan already planned.',
      canConfirm: true,
    })
  })

  it('states one week once when several cells land there', () => {
    const sum: PlanSummary = {
      cells: [filled('2026-12-28'), filled('2026-12-28')],
      filled: 2,
      skipped: 0,
      skipped_detail: [],
    }
    expect(bulkPreview('seed', sum).notice).toBe('Would seed 2 cells into 28 Dec.')
  })

  it('says one cell, not one cells', () => {
    const sum: PlanSummary = { cells: [filled('2026-12-28')], filled: 1, skipped: 0, skipped_detail: [] }
    expect(bulkPreview('copy', sum).notice).toBe('Would copy 1 cell into 28 Dec.')
  })

  it('offers no confirm when every week would be skipped, and says why', () => {
    const sum: PlanSummary = {
      cells: [],
      filled: 0,
      skipped: 2,
      skipped_detail: [skip('2027-01-04', 'frozen'), skip('2027-01-11', 'archived')],
    }
    expect(bulkPreview('copy', sum)).toEqual({
      notice: 'Nothing to copy — 4 Jan week has ended, 11 Jan project is archived.',
      canConfirm: false,
    })
  })

  it('offers no confirm when a run would change nothing at all', () => {
    expect(bulkPreview('seed', undefined)).toEqual({
      notice: 'Nothing to seed: no cell would change.',
      canConfirm: false,
    })
  })

  it('keeps the cells count when the server listed no target weeks', () => {
    const sum: PlanSummary = { cells: null, filled: 3, skipped: 0, skipped_detail: null }
    expect(bulkPreview('copy', sum).notice).toBe('Would copy 3 cells.')
  })
})

describe('ended weeks', () => {
  it('judges the week in the planned person timezone, not the viewer one', () => {
    const now = new Date('2026-11-09T00:30:00Z')
    expect(weekEnded('2026-11-02', 'Pacific/Kiritimati', now)).toBe(true)
    expect(weekEnded('2026-11-02', 'Pacific/Midway', now)).toBe(false)
  })

  it('keeps the current week editable on both sides of the boundary', () => {
    const now = new Date('2026-11-10T12:00:00Z')
    expect(weekEnded('2026-11-09', 'Pacific/Kiritimati', now)).toBe(false)
    expect(weekEnded('2026-11-09', 'Pacific/Midway', now)).toBe(false)
  })

  it('turns over exactly at midnight Monday in that zone', () => {
    const at = (instant: string) => weekEnded('2026-11-02', 'Pacific/Kiritimati', new Date(instant))
    expect(at('2026-11-08T09:59:00Z')).toBe(false)
    expect(at('2026-11-08T10:00:00Z')).toBe(true)
  })

  it('falls back to UTC for a timezone it cannot read, like the server', () => {
    expect(weekEnded('2026-11-02', 'Not/AZone', new Date('2026-11-09T00:00:00Z'))).toBe(true)
    expect(weekEnded('2026-11-02', '', new Date('2026-11-08T23:59:00Z'))).toBe(false)
  })
})

describe('assignable projects', () => {
  it('drops archived projects, which cannot take plan hours', () => {
    const projects = [project(1, null), project(2, '2026-11-01T14:00:00+01:00'), project(3, null)]
    expect(assignableProjects(projects, person([1])).map((p) => p.id)).toEqual([1, 3])
  })

  it('keeps every project when none is archived', () => {
    const projects = [project(1, null), project(2, null)]
    expect(assignableProjects(projects, person([1])).map((p) => p.id)).toEqual([1, 2])
  })

  it('drops projects of teams the person is not in', () => {
    const projects = [project(1, null, 1), project(2, null, 2), project(3, null, 1)]
    expect(assignableProjects(projects, person([1])).map((p) => p.id)).toEqual([1, 3])
  })

  it('offers nothing to a person with no teams', () => {
    expect(assignableProjects([project(1, null)], person(null))).toEqual([])
    expect(assignableProjects([project(1, null)], undefined)).toEqual([])
  })

  it('has nothing to offer when there are no projects', () => {
    expect(assignableProjects([], person([1]))).toEqual([])
  })
})
