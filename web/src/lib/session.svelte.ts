import { api, problemMessage } from './api'
import type { components } from './api/schema'

export type Me = components['schemas']['MeResponse']

let user = $state<Me | null>(null)

export const session = {
  get user(): Me | null {
    return user
  },
  get mustChangePassword(): boolean {
    return user?.must_change_password ?? false
  },

  // load fetches the signed-in user; a 401 is an anonymous session, not an error.
  async load(): Promise<Me | null> {
    const { data, response } = await api.GET('/api/v1/me')
    if (data) {
      user = data
      return user
    }
    user = null
    if (response.status !== 401) throw new Error(`could not load session (${response.status})`)
    return null
  },

  async login(email: string, password: string): Promise<void> {
    const { data, error, response } = await api.POST('/api/v1/auth/login', {
      body: { email, password },
    })
    if (error) throw new Error(problemMessage(error, response))
    user = data ?? null
  },

  async logout(): Promise<void> {
    const { error, response } = await api.POST('/api/v1/auth/logout')
    // 401 means the session is already gone; that is a successful logout.
    if (error && response.status !== 401) throw new Error(problemMessage(error, response))
    user = null
  },
}
