import { api } from './index.js'

function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getClusterDNS(options) { return api.get('/cluster-dns', options) }
export function updateClusterDNS(body, options) { return post('/cluster-dns', body, options) }
export function deleteClusterDNS(options) { return remove('/cluster-dns', options) }
export function rollbackClusterDNS(revision, options) {
  return post(`/cluster-dns/history/${encodeURIComponent(revision)}/rollback`, undefined, options)
}
