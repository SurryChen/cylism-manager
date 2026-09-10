import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getLoggingFilters, getLoggingStatus, installLogging, queryLogs, saveLoggingConfig, uninstallLogging } from './logging.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

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

  it('keeps logging mutation payloads separate from request options', () => {
    const options = { signal: new AbortController().signal }
    const body = { node_name: 'node-a', retention_days: 14 }
    installLogging(body, options)
    saveLoggingConfig(body, options)
    uninstallLogging(options)

    expect(api.post).toHaveBeenCalledWith('/monitoring/logs/install', body, options)
    expect(api.put).toHaveBeenCalledWith('/monitoring/logs/config', body, options)
    expect(api.delete).toHaveBeenCalledWith('/monitoring/logs', undefined, options)
  })
})
