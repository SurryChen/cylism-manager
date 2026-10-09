import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getMonitoringStatus, getMonitoringNodes, getStorageClasses, getMonitoringDashboard, installMonitoring } from './monitoring.js'
import { getLoggingFilters, queryLogs } from './logging.js'
import { getAlertingOverview, createAlertingSilence } from './alerting.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('cluster observability API', () => {
  afterEach(() => vi.clearAllMocks())

  it('routes metric and inventory requests to the selected cluster', () => {
    const options = { clusterID: 2, signal: new AbortController().signal }
    getMonitoringStatus(options)
    getMonitoringNodes(options)
    getStorageClasses(options)
    getMonitoringDashboard('6h', options)
    installMonitoring({ node_name: 'node-a' }, options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/clusters/2/monitoring/status', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/nodes?cluster_id=2', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/clusters/2/k8s/storage-classes', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/clusters/2/monitoring/dashboard?range=6h', options)
    expect(api.post).toHaveBeenCalledWith('/clusters/2/monitoring/install', { node_name: 'node-a' }, options)
  })

  it('routes logs and alert actions to the selected cluster', () => {
    const options = { clusterID: 7 }
    getLoggingFilters(options)
    queryLogs({ range: '1h' }, options)
    getAlertingOverview(options)
    createAlertingSilence({ comment: 'maintenance' }, options)
    expect(api.get).toHaveBeenCalledWith('/clusters/7/monitoring/logs/filters', options)
    expect(api.post).toHaveBeenCalledWith('/clusters/7/monitoring/logs/query', { range: '1h' }, options)
    expect(api.get).toHaveBeenCalledWith('/clusters/7/monitoring/alerts/overview', options)
    expect(api.post).toHaveBeenCalledWith('/clusters/7/monitoring/alerts/silences', { comment: 'maintenance' }, options)
  })
})
