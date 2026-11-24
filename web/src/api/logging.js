import { api } from './index.js'

const loggingPath = options => `${options?.clusterID ? `/clusters/${encodeURIComponent(options.clusterID)}` : ''}/monitoring/logs`

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

export function getLoggingStatus(options) { return api.get(`${loggingPath(options)}/status`, options) }
export function getLoggingFilters(options) { return api.get(`${loggingPath(options)}/filters`, options) }
export function queryLogs(payload, options) { return api.post(`${loggingPath(options)}/query`, payload, options) }
export function installLogging(body, options) { return post(`${loggingPath(options)}/install`, body, options) }
export function saveLoggingConfig(body, options) { return put(`${loggingPath(options)}/config`, body, options) }
export function uninstallLogging(options) { return remove(loggingPath(options), options) }
