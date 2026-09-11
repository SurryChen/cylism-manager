import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createAlertingSilence, getAlertingAutomationEvents, getAlertingOverview, getAlertingStatus, installAlerting, saveAlertingAutomationPolicy, saveAlertingConfig, testAlertingNotification } from './alerting.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))

describe('alerting api', () => {
  it('forwards request options for managed reads', () => {
    const options = { signal: new AbortController().signal }
    getAlertingStatus(options)
    getAlertingOverview(options)
    getAlertingAutomationEvents(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/monitoring/alerts/status', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/monitoring/alerts/overview', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/monitoring/alerts/automation-events', options)
  })

  it('preserves alerting mutation contracts and notification query encoding', () => {
    const options = { signal: new AbortController().signal }
    const body = { node_name: 'node-a' }
    installAlerting(body, options)
    createAlertingSilence(body, options)
    saveAlertingAutomationPolicy(body, options)
    saveAlertingConfig(body, options)
    testAlertingNotification('feishu webhook', options)

    expect(api.post).toHaveBeenNthCalledWith(1, '/monitoring/alerts/install', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/monitoring/alerts/silences', body, options)
    expect(api.put).toHaveBeenNthCalledWith(1, '/monitoring/alerts/automation-policy', body, options)
    expect(api.put).toHaveBeenNthCalledWith(2, '/monitoring/alerts/config', body, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/monitoring/alerts/test-notification?channel=feishu%20webhook', {}, options)
  })
})
