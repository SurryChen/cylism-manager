import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getAlertOverview, getDashboardOverview, getKubernetesDashboard } from './dashboard.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('dashboard api', () => {
  afterEach(() => vi.clearAllMocks())

  it('maps dashboard reads to their stable endpoints', () => {
    const options = { signal: new AbortController().signal }

    getDashboardOverview(options)
    getKubernetesDashboard(options)
    getAlertOverview(options)

    expect(api.get).toHaveBeenNthCalledWith(1, '/dashboard', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/k8s/dashboard', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/monitoring/alerts/overview', options)
  })
})
