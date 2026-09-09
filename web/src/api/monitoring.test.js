import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getMonitoringDashboard, queryMonitoring } from './monitoring.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('monitoring API', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes monitoring parameters and forwards the abort signal', () => {
    const signal = new AbortController().signal

    getMonitoringDashboard('24h', { signal })
    queryMonitoring('up{job="node"}', { signal })

    expect(api.get).toHaveBeenNthCalledWith(1, '/monitoring/dashboard?range=24h', { signal })
    expect(api.get).toHaveBeenNthCalledWith(2, '/monitoring/query?query=up%7Bjob%3D%22node%22%7D', { signal })
  })
})
