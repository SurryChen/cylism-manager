import { api } from './index.js'

const monitoringPath = options => options?.clusterID ? `/clusters/${encodeURIComponent(options.clusterID)}/monitoring` : '/monitoring'

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
  return api.get(`${monitoringPath(options)}/status`, options)
}

export function getMonitoringNodes(options) {
  return api.get(options?.clusterID ? `/nodes?cluster_id=${encodeURIComponent(options.clusterID)}` : '/nodes', options)
}

export function getStorageClasses(options) {
  return api.get(options?.clusterID ? `/clusters/${encodeURIComponent(options.clusterID)}/k8s/storage-classes` : '/k8s/storage-classes', options)
}

export function getMonitoringTargets(options) {
  return api.get(`${monitoringPath(options)}/targets`, options)
}

export function getMonitoringDashboard(range, options) {
  return api.get(`${monitoringPath(options)}/dashboard?range=${encodeURIComponent(range)}`, options)
}

export function queryMonitoring(query, options) {
  return api.get(`${monitoringPath(options)}/query?query=${encodeURIComponent(query)}`, options)
}

export function getDiskGrowth({ range, node } = {}, options) {
  const params = new URLSearchParams({ range })
  if (node) params.set('node', node)
  return api.get(`${monitoringPath(options)}/disk-growth?${params.toString()}`, options)
}

export function installMonitoring(body, options) { return post(`${monitoringPath(options)}/install`, body, options) }
export function uninstallMonitoring(options) { return remove(monitoringPath(options), undefined, options) }
export function migrateMonitoringStorage(body, options) { return post(`${monitoringPath(options)}/storage-migration`, body, options) }
