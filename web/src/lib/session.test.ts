import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('./api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api')>()
  return { ...actual, api: { ...actual.api, POST: post } }
})

import { session } from './session.svelte'

const me = {
  id: 1,
  email: 'ada@example.com',
  name: 'Ada',
  timezone: 'UTC',
  weekly_hours: 40,
  is_admin: false,
  must_change_password: false,
  memberships: [],
}

beforeEach(() => {
  post.mockReset()
})

describe('session.logout', () => {
  it('clears the user when the server ends the session', async () => {
    post.mockResolvedValueOnce({ data: me, response: new Response(null, { status: 200 }) })
    await session.login('ada@example.com', 'secret123')
    post.mockResolvedValueOnce({ response: new Response(null, { status: 204 }) })

    await session.logout()

    expect(post).toHaveBeenLastCalledWith('/api/v1/auth/logout')
    expect(session.user).toBeNull()
  })

  it('keeps the user when the server fails to end the session', async () => {
    post.mockResolvedValueOnce({ data: me, response: new Response(null, { status: 200 }) })
    await session.login('ada@example.com', 'secret123')
    post.mockResolvedValueOnce({
      error: { type: 'urn:tstamps:error:internal', detail: 'internal error' },
      response: new Response(null, { status: 500 }),
    })

    await expect(session.logout()).rejects.toThrow('internal error')
    expect(session.user).toEqual(me)
  })

  it('clears the user when the session already ended', async () => {
    post.mockResolvedValueOnce({ data: me, response: new Response(null, { status: 200 }) })
    await session.login('ada@example.com', 'secret123')
    post.mockResolvedValueOnce({
      error: { type: 'urn:tstamps:error:unauthorized', detail: 'authentication required' },
      response: new Response(null, { status: 401 }),
    })

    await session.logout()

    expect(session.user).toBeNull()
  })
})
