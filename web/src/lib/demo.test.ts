import { describe, expect, it } from 'vitest'
import { parseAccounts } from './demo'

describe('parseAccounts', () => {
  it('returns nothing without a marker', () => {
    expect(parseAccounts(null)).toEqual([])
    expect(parseAccounts('')).toEqual([])
  })

  it('returns nothing for malformed or non-array content', () => {
    expect(parseAccounts('not json')).toEqual([])
    expect(parseAccounts('{"email":"ada@example.com"}')).toEqual([])
  })

  it('reads the demo accounts', () => {
    const accounts = [{ email: 'ada@example.com', name: 'Ada', role: 'admin', password: 'demo' }]
    expect(parseAccounts(JSON.stringify(accounts))).toEqual(accounts)
  })
})
