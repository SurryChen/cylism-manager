import { api } from './index.js'

export function getAlertingStatus(options) { return api.get('/monitoring/alerts/status', options) }
export function getAlertingOverview(options) { return api.get('/monitoring/alerts/overview', options) }
export function getAlertingAutomationPolicy(options) { return api.get('/monitoring/alerts/automation-policy', options) }
export function getAlertingAutomationEvents(options) { return api.get('/monitoring/alerts/automation-events', options) }
export function getAlertingSilences(options) { return api.get('/monitoring/alerts/silences', options) }
