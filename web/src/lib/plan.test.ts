import { describe, expect, it } from 'vitest'
import { monthRange, nextMonth, periodLabel, planRange, previousMonth, weekLabel } from './plan'

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
