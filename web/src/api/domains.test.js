import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getDomainOptions, getManagedDomains } from './domains.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('domains api', () => {
  it('builds managed domain filters', () => {
    const options = { signal: new AbortController().signal }
    getManagedDomains({ environmentID: 2, unassigned: false }, options)
    expect(api.get).toHaveBeenCalledWith('/domains?environment_id=2', options)
  })

  it('forwards options for domain options', () => {
    const options = { signal: new AbortController().signal }
    getDomainOptions(options)
    expect(api.get).toHaveBeenCalledWith('/certs/issuers', options)
  })
})
