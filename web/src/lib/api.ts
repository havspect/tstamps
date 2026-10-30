import createClient from 'openapi-fetch'
import type { paths } from './api/schema'

const workspace = /^\/w\/([^/]+)/.exec(location.pathname)
export const workspaceBase = workspace ? `/w/${workspace[1]}` : ''

export const api = createClient<paths>({ baseUrl: workspaceBase })

// URNs are the stable part of a problem response; detail may be reworded.
const urnMessages: Record<string, string> = {
  'invalid-credentials': 'Email or password is incorrect.',
  'password-change-required': 'Change your password before continuing.',
  'timer-already-running': 'A timer is already running. Stop it first.',
  'entry-overlap': 'Another entry already covers that time on this project.',
  'entry-too-long': 'An entry may not be longer than 24 hours.',
  'last-admin': 'The last active admin cannot be demoted or deactivated.',
  'project-has-entries': 'A project with entries cannot change team.',
  'invalid-input': 'That input is not valid.',
  forbidden: 'You do not have permission for that.',
  'not-found': 'Not found.',
  'rate-limited': 'Too many attempts. Try again later.',
  'invalid-origin': 'The browser origin was rejected. Reload and try again.',
}

type Problem = { type?: string; title?: string; detail?: string }

// problemMessage turns an RFC 9457 body into prose. Detail wins; the URN map is
// the fallback when a proxy or an old server sends a bare problem.
export function problemMessage(error: unknown, response?: Response): string {
  const problem = (error ?? {}) as Problem
  if (problem.detail) return problem.detail
  const urn = problem.type?.replace(/^urn:tstamps:error:/, '')
  if (urn && urnMessages[urn]) return urnMessages[urn]
  return problem.title ?? (response ? `Request failed (${response.status}).` : 'Something went wrong.')
}
