import { api } from './index.js'

const alertingPath = options => `${options?.clusterID ? `/clusters/${encodeURIComponent(options.clusterID)}` : ''}/monitoring/alerts`

function post(path, body, options) {
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function put(path, body, options) {
  if (options === undefined) return api.put(path, body)
  return api.put(path, body, options)
}

export function getAlertingStatus(options) { return api.get(`${alertingPath(options)}/status`, options) }
export function getAlertingOverview(options) { return api.get(`${alertingPath(options)}/overview`, options) }
export function getAlertingAutomationPolicy(options) { return api.get('/monitoring/alerts/automation-policy', options) }
export function getAlertingAutomationEvents(options) { return api.get('/monitoring/alerts/automation-events', options) }
export function getAlertingSilences(options) { return api.get(`${alertingPath(options)}/silences`, options) }
export function installAlerting(body, options) { return post(`${alertingPath(options)}/install`, body, options) }
export function createAlertingSilence(body, options) { return post(`${alertingPath(options)}/silences`, body, options) }
export function saveAlertingAutomationPolicy(body, options) { return put('/monitoring/alerts/automation-policy', body, options) }
export function testAlertingNotification(channel, options) { return post(`${alertingPath(options)}/test-notification?channel=${encodeURIComponent(channel)}`, {}, options) }
export function saveAlertingConfig(body, options) { return put(`${alertingPath(options)}/config`, body, options) }

export function getAlertSuppressionRules({ clusterID, node = '', status = '', limit = 10, offset = 0 } = {}, options) {
  const params = new URLSearchParams({ node, status, limit: String(limit), offset: String(offset) })
  return api.get(`${alertingPath({ clusterID })}/suppression-rules?${params}`, options)
}
export function createAlertSuppressionRule(body, options) { return post(`${alertingPath(options)}/suppression-rules`, body, options) }
export function updateAlertSuppressionRule(id, body, options) { return put(`${alertingPath(options)}/suppression-rules/${encodeURIComponent(id)}`, body, options) }
export function setAlertSuppressionEnabled(id, enabled, options) { return api.patch(`${alertingPath(options)}/suppression-rules/${encodeURIComponent(id)}/enabled`, { enabled }, options) }
export function deleteAlertSuppressionRule(id, options) { return api.delete(`${alertingPath(options)}/suppression-rules/${encodeURIComponent(id)}`, undefined, options) }

export function getAlertingHistory({ clusterID, limit = 10, offset = 0, view = 'history', node = '', alertName = '', days = '7', status = '' } = {}, options) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset), view, days: String(days) })
  if (node) params.set('node', node)
  if (alertName) params.set('alert_name', alertName)
  if (status) params.set('status', status)
  return api.get(`${alertingPath({ clusterID })}/history?${params}`, options)
}
