import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getAuditLogs } from './audit.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('audit api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes structured filters and request options', () => {
    const options = { signal: new AbortController().signal }
    getAuditLogs({ limit: 20, offset: 40, resourceType: 'agent runtime', action: 'deploy', outcome: 'failed', source: 'agent', actorType: 'user', targetName: 'console', keyword: 'a/b' }, options)
    expect(api.get).toHaveBeenCalledWith('/audit-logs?limit=20&offset=40&resource_type=agent+runtime&action=deploy&outcome=failed&source=agent&actor_type=user&target_name=console&keyword=a%2Fb', options)
  })

  it('encodes date range filters', () => {
    getAuditLogs({ createdFrom: '2026-09-01', createdTo: '2026-09-08' })
    expect(api.get).toHaveBeenCalledWith('/audit-logs?limit=20&offset=0&created_from=2026-09-01&created_to=2026-09-08', undefined)
  })
})
