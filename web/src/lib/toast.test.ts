import { afterEach, describe, expect, it, vi } from 'vitest'
import { notify, notifyError } from './toast'

// Vitest runs in node, which has no document: stand one in to record the call.
function stubToast(found = true) {
  const create = vi.fn()
  vi.stubGlobal('document', { querySelector: () => (found ? { create } : null) })
  return create
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('notify', () => {
  it('is a no-op without a document', () => {
    expect(() => notify('Saved')).not.toThrow()
  })

  it('is a no-op when the page has no wa-toast', () => {
    const create = stubToast(false)
    expect(() => notify('Saved')).not.toThrow()
    expect(create).not.toHaveBeenCalled()
  })

  it('shows a success toast for 5 s with an icon', () => {
    const create = stubToast()
    notify('Saved', { variant: 'success' })
    expect(create).toHaveBeenCalledWith('Saved', {
      variant: 'success',
      duration: 5000,
      icon: 'circle-check',
    })
  })

  it('keeps duration 0 as sticky', () => {
    const create = stubToast()
    notify('Saving', { duration: 0 })
    expect(create).toHaveBeenCalledWith('Saving', {
      variant: 'neutral',
      duration: 0,
      icon: undefined,
    })
  })
})

describe('notifyError', () => {
  it('shows the server detail verbatim for 8 s', () => {
    const create = stubToast()
    notifyError({ type: 'urn:tstamps:error:entry-overlap', detail: 'That time is already taken.' })
    expect(create).toHaveBeenCalledWith('That time is already taken.', {
      variant: 'danger',
      duration: 8000,
      icon: 'circle-exclamation',
    })
  })

  it('falls back to generic text for a network failure', () => {
    const create = stubToast()
    notifyError(new TypeError('Failed to fetch'))
    expect(create.mock.calls[0][0]).toBe('Something went wrong.')
  })

  it('is a no-op without a document', () => {
    expect(() => notifyError(new Error('x'))).not.toThrow()
  })
})
