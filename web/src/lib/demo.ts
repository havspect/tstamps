// The demo shell carries its public sign-ins as a meta tag; other servers do
// not, so every check below is simply "is this a demo".

export type DemoAccount = {
  email: string
  name: string
  role: string
  password: string
}

export function parseAccounts(content: string | null | undefined): DemoAccount[] {
  if (!content) return []
  try {
    const parsed: unknown = JSON.parse(content)
    return Array.isArray(parsed) ? (parsed as DemoAccount[]) : []
  } catch {
    return []
  }
}

function readAccounts(): DemoAccount[] {
  if (typeof document === 'undefined') return []
  return parseAccounts(document.querySelector<HTMLMetaElement>('meta[name="tstamps-demo"]')?.content)
}

export const demo = { accounts: readAccounts() }
