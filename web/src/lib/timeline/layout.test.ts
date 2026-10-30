import { describe, expect, it } from 'vitest'
import { FINE_SNAP, SNAP, clampRange, findOverlap, orderProjects, snap, timeToX, xToTime } from './layout'

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const NOW = Date.parse('2026-10-23T12:00:00Z')

describe('timeToX and xToTime', () => {
  it('round trips', () => {
    const origin = NOW
    const msPerPx = DAY / 200
    const x = timeToX(origin + 6 * HOUR, origin, msPerPx)
    expect(x).toBe(50)
    expect(xToTime(x, origin, msPerPx)).toBe(origin + 6 * HOUR)
  })
})

describe('snap', () => {
  it('rounds to the nearest quarter hour', () => {
    expect(snap(NOW + 8 * MIN, SNAP)).toBe(NOW + 15 * MIN)
    expect(snap(NOW + 7 * MIN, SNAP)).toBe(NOW)
    expect(snap(NOW + 22 * MIN, SNAP)).toBe(NOW + 15 * MIN)
  })

  it('rounds to the nearest minute when fine', () => {
    expect(snap(NOW + 29_000, FINE_SNAP)).toBe(NOW)
    expect(snap(NOW + 31_000, FINE_SNAP)).toBe(NOW + MIN)
  })

  it('can be switched off', () => {
    expect(snap(1234, SNAP, false)).toBe(1234)
  })
})

describe('clampRange', () => {
  it('ends no later than now plus the grace', () => {
    const end = NOW + 3 * HOUR
    const range = clampRange(end - HOUR, end, NOW)
    expect(range.end).toBe(NOW + 5 * MIN)
    expect(range.start).toBe(NOW + 5 * MIN - HOUR)
  })

  it('keeps a moved block its full duration', () => {
    const range = clampRange(NOW - 1 * HOUR, NOW + 4 * HOUR, NOW, { moving: true })
    expect(range.end).toBe(NOW + 5 * MIN)
    expect(range.end - range.start).toBe(5 * HOUR)
  })

  it('caps a too-long span at 24 hours', () => {
    const range = clampRange(NOW - 25 * HOUR, NOW, NOW)
    expect(range.start).toBe(NOW - 24 * HOUR)
    expect(range.end).toBe(NOW)
  })

  it('pushes the start to within 24 hours of the end', () => {
    const range = clampRange(NOW - 30 * HOUR, NOW - 2 * HOUR, NOW)
    expect(range.end).toBe(NOW - 2 * HOUR)
    expect(range.start).toBe(NOW - 26 * HOUR)
  })

  it('keeps at least one minute', () => {
    const point = clampRange(NOW, NOW, NOW)
    expect(point.end - point.start).toBe(MIN)

    const backwards = clampRange(NOW - 2 * HOUR, NOW - 3 * HOUR, NOW)
    expect(backwards.end - backwards.start).toBe(HOUR)
  })
})

describe('orderProjects', () => {
  it('orders projects by their first block, then the empty remembered ones', () => {
    const order = orderProjects(
      [
        { projectID: 3, start: NOW + 2 * HOUR },
        { projectID: 1, start: NOW },
        { projectID: 3, start: NOW + 5 * HOUR },
      ],
      [2, 1],
    )
    expect(order).toEqual([1, 3, 2])
  })

  it('lists a project once, even with many blocks', () => {
    const order = orderProjects(
      [
        { projectID: 4, start: NOW },
        { projectID: 4, start: NOW + HOUR },
      ],
      [4],
    )
    expect(order).toEqual([4])
  })

  it('follows the remembered order when nothing has time', () => {
    expect(orderProjects([], [7, 2, 7])).toEqual([7, 2])
  })
})

describe('findOverlap', () => {
  const at = (h: number, m = 0) => NOW + h * HOUR + m * MIN
  const spans = [
    { id: 1, projectID: 7, start: at(9), end: at(10) },
    { id: 2, projectID: 7, start: at(11), end: Number.POSITIVE_INFINITY },
    { id: 3, projectID: 8, start: at(9), end: at(10) },
  ]

  it('finds a partial or enclosed overlap', () => {
    expect(findOverlap(spans, 7, at(9, 30), at(10, 30))?.id).toBe(1)
    expect(findOverlap(spans, 7, at(9, 15), at(9, 45))?.id).toBe(1)
  })

  it('treats touching edges as free', () => {
    expect(findOverlap(spans, 7, at(10), at(11))).toBeNull()
  })

  it('treats a running entry as open-ended', () => {
    expect(findOverlap(spans, 7, at(15), at(16))?.id).toBe(2)
    expect(findOverlap(spans, 7, at(8), at(9))).toBeNull()
  })

  it('ignores other projects and the entry being edited', () => {
    expect(findOverlap(spans, 7, at(9), at(10), 1)).toBeNull()
    expect(findOverlap(spans, 9, at(9), at(10))).toBeNull()
  })
})
