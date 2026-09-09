import { api } from './index.js'

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
