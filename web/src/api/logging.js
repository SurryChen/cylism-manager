import { api } from './index.js'

function post(path, body, options) {
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function put(path, body, options) {
  if (options === undefined) return api.put(path, body)
  return api.put(path, body, options)
}

function remove(path, options) {
  if (options === undefined) return api.delete(path)
  return api.delete(path, undefined, options)
}

export function getLoggingStatus(options) { return api.get('/monitoring/logs/status', options) }
export function getLoggingFilters(options) { return api.get('/monitoring/logs/filters', options) }
export function queryLogs(payload, options) { return api.post('/monitoring/logs/query', payload, options) }
export function installLogging(body, options) { return post('/monitoring/logs/install', body, options) }
export function saveLoggingConfig(body, options) { return put('/monitoring/logs/config', body, options) }
export function uninstallLogging(options) { return remove('/monitoring/logs', options) }
