import { api } from './index.js'

export function getLoggingStatus(options) { return api.get('/monitoring/logs/status', options) }
export function getLoggingFilters(options) { return api.get('/monitoring/logs/filters', options) }
export function queryLogs(payload, options) { return api.post('/monitoring/logs/query', payload, options) }
