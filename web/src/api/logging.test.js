import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getLoggingFilters, getLoggingStatus, queryLogs } from './logging.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('logging api', () => {
  it('forwards options to reads and queries', () => {
    const options = { signal: new AbortController().signal }
    getLoggingStatus(options)
    getLoggingFilters(options)
    queryLogs({ node: 'node-a' }, options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/monitoring/logs/status', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/monitoring/logs/filters', options)
    expect(api.post).toHaveBeenCalledWith('/monitoring/logs/query', { node: 'node-a' }, options)
  })
})
