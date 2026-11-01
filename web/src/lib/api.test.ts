import { describe, expect, it } from 'vitest'
import { problemMessage } from './api'

describe('problemMessage', () => {
  it('prefers a detail that says something', () => {
    expect(problemMessage({ type: 'urn:tstamps:error:plan-week-frozen', detail: 'the week has ended' })).toBe(
      'the week has ended',
    )
  })

  it('falls back to the URN sentence when the detail only repeats the slug', () => {
    expect(problemMessage({ type: 'urn:tstamps:error:forbidden', detail: 'forbidden' })).toBe(
      'You do not have permission for that.',
    )
  })

  it('falls back to the title, then to the response status', () => {
    expect(problemMessage({ title: 'Unprocessable Entity' })).toBe('Unprocessable Entity')
    expect(problemMessage(undefined, new Response(null, { status: 500 }))).toBe('Request failed (500).')
    expect(problemMessage(undefined)).toBe('Something went wrong.')
  })
})
