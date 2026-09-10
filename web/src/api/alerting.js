import { api } from './index.js'

function post(path, body, options) {
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function put(path, body, options) {
  if (options === undefined) return api.put(path, body)
  return api.put(path, body, options)
}

export function getAlertingStatus(options) { return api.get('/monitoring/alerts/status', options) }
export function getAlertingOverview(options) { return api.get('/monitoring/alerts/overview', options) }
export function getAlertingAutomationPolicy(options) { return api.get('/monitoring/alerts/automation-policy', options) }
export function getAlertingAutomationEvents(options) { return api.get('/monitoring/alerts/automation-events', options) }
export function getAlertingSilences(options) { return api.get('/monitoring/alerts/silences', options) }
export function installAlerting(body, options) { return post('/monitoring/alerts/install', body, options) }
export function createAlertingSilence(body, options) { return post('/monitoring/alerts/silences', body, options) }
export function saveAlertingAutomationPolicy(body, options) { return put('/monitoring/alerts/automation-policy', body, options) }
export function testAlertingNotification(channel, options) { return post(`/monitoring/alerts/test-notification?channel=${encodeURIComponent(channel)}`, {}, options) }
export function saveAlertingConfig(body, options) { return put('/monitoring/alerts/config', body, options) }
