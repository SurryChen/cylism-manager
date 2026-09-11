import { api } from './index.js'

function post(path, body, options) {
  if (body === undefined && options === undefined) return api.post(path)
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function remove(path, body, options) {
  if (body === undefined && options === undefined) return api.delete(path)
  if (options === undefined) return api.delete(path, body)
  return api.delete(path, body, options)
}

export function getMonitoringStatus(options) {
  return api.get('/monitoring/status', options)
}

export function getMonitoringNodes(options) {
  return api.get('/nodes', options)
}

export function getStorageClasses(options) {
  return api.get('/k8s/storage-classes', options)
}

export function getMonitoringTargets(options) {
  return api.get('/monitoring/targets', options)
}

export function getMonitoringDashboard(range, options) {
  return api.get(`/monitoring/dashboard?range=${encodeURIComponent(range)}`, options)
}

export function queryMonitoring(query, options) {
  return api.get(`/monitoring/query?query=${encodeURIComponent(query)}`, options)
}

export function getDiskGrowth({ range, node } = {}, options) {
  const params = new URLSearchParams({ range })
  if (node) params.set('node', node)
  return api.get(`/monitoring/disk-growth?${params.toString()}`, options)
}

export function installMonitoring(body, options) { return post('/monitoring/install', body, options) }
export function uninstallMonitoring(options) { return remove('/monitoring', undefined, options) }
export function migrateMonitoringStorage(body, options) { return post('/monitoring/storage-migration', body, options) }
