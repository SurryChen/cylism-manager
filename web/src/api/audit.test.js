import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getAuditLogs } from './audit.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('audit api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes pagination, filters, sort, and request options', () => {
    const options = { signal: new AbortController().signal }
    getAuditLogs({ limit: 20, offset: 40, resourceType: 'agent runtime', action: 'deploy', keyword: 'a/b', sort: 'created_at', order: 'desc' }, options)
    expect(api.get).toHaveBeenCalledWith('/audit-logs?limit=20&offset=40&resource_type=agent+runtime&action=deploy&keyword=a%2Fb&sort=created_at&order=desc', options)
  })
})
