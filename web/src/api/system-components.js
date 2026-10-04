import { api } from './index.js'
export { getClusterNodes } from './cluster.js'

function scopedPath(path, clusterID) {
  return clusterID === undefined || clusterID === null || clusterID === ''
    ? path
    : `${path}?cluster_id=${encodeURIComponent(clusterID)}`
}

export function getSystemComponents(options, clusterID) { return api.get(scopedPath('/system-components', clusterID), options) }

function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function post(path, options) { return options ? api.post(path, undefined, options) : api.post(path) }

export function updateSystemComponent(chartName, payload, options, clusterID) {
  return put(scopedPath(`/system-components/${encodeURIComponent(chartName)}`, clusterID), payload, options)
}

export function revertSystemComponent(chartName, options, clusterID) {
  return post(scopedPath(`/system-components/${encodeURIComponent(chartName)}/revert`, clusterID), options)
}
