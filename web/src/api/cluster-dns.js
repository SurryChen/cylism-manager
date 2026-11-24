import { api } from './index.js'

function scopedPath(path, clusterID) {
  return clusterID === undefined || clusterID === null || clusterID === ''
    ? path
    : `${path}${path.includes('?') ? '&' : '?'}cluster_id=${encodeURIComponent(clusterID)}`
}

function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getClusterDNS(options, clusterID) { return api.get(scopedPath('/cluster-dns', clusterID), options) }
export function updateClusterDNS(body, options, clusterID) { return post(scopedPath('/cluster-dns', clusterID), body, options) }
export function deleteClusterDNS(options, clusterID) { return remove(scopedPath('/cluster-dns', clusterID), options) }
export function rollbackClusterDNS(revision, options, clusterID) {
  return post(scopedPath(`/cluster-dns/history/${encodeURIComponent(revision)}/rollback`, clusterID), undefined, options)
}
