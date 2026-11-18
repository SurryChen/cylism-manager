import { api } from './index.js'

function queryPath(path, query) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== '') params.set(key, String(value))
  return path + '?' + params
}
export function getInventoryPage(clusterID, kind, query, options) {
  const prefix = clusterID ? '/clusters/' + encodeURIComponent(clusterID) + '/k8s' : '/k8s'
  return api.get(queryPath(prefix + '/' + kind, query), options)
}
export function getCertificatePage(clusterID, query, options) {
  const path = clusterID ? '/clusters/' + encodeURIComponent(clusterID) + '/k8s/certs' : '/certs'
  return api.get(queryPath(path, query), options)
}
export function getWorkspaceReleasePage(query, options) { return api.get(queryPath('/workspace/releases', query), options) }
export function getApplicationReleasePage(applicationID, query, options) { return api.get(queryPath('/applications/' + encodeURIComponent(applicationID) + '/releases', query), options) }

export function getWorkspacePage(query, options) { return api.get(queryPath('/workspace/overview', query), options) }

export function getListPage(path, query, options) { return api.get(queryPath(path, query), options) }
