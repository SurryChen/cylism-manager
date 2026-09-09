import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getAlertingAutomationEvents, getAlertingOverview, getAlertingStatus } from './alerting.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

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
})
