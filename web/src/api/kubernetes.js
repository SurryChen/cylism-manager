import { api } from './index.js'

function namespaceQuery(namespace) {
  return namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''
}

export function getResourcePods(namespace = '', options) {
  return api.get(`/k8s/pods${namespaceQuery(namespace)}`, options)
}

export function getResourceServices(namespace = '', options) {
  return api.get(`/k8s/services${namespaceQuery(namespace)}`, options)
}

export function getResourceDeployments(namespace = '', options) {
  return api.get(`/k8s/deployments${namespaceQuery(namespace)}`, options)
}

function get(path, options) { return options ? api.get(path, options) : api.get(path) }
function post(path, body, options) { return options ? api.post(path, body, options) : api.post(path, body) }
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getWorkloadDeployments(options) { return get('/k8s/deployments', options) }
export function getWorkloadStatefulSets(options) { return get('/k8s/statefulsets', options) }
export function getWorkloadDaemonSets(options) { return get('/k8s/daemonsets', options) }
export function getWorkloadPods(options) { return get('/k8s/pods', options) }
export function getWorkloadServers(options) { return get('/servers', options) }
export function getWorkloadDeploymentPods(namespace, name, options) {
  return get(`/k8s/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/pods`, options)
}
export function getWorkloadDeploymentRevisions(namespace, name, options) {
  return get(`/k8s/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/revisions`, options)
}
export function scaleWorkload(kind, namespace, name, replicas, options) {
  const resource = kind === 'statefulset' ? 'statefulsets' : 'deployments'
  return options ? api.patch(`/k8s/${resource}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/scale`, { replicas }, options) : api.patch(`/k8s/${resource}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/scale`, { replicas })
}
export function updateWorkloadImage(namespace, name, payload, options) {
  return options ? api.patch(`/k8s/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/image`, payload, options) : api.patch(`/k8s/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/image`, payload)
}
export function rollbackWorkload(namespace, name, revision, options) {
  return post(`/k8s/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/rollback`, { revision }, options)
}
export function getConfigMaps(options) { return get('/k8s/configmaps', options) }
export function getSecrets(options) { return get('/k8s/secrets', options) }
export function getConfigMapsForNamespace(namespace, options) {
  return get(`/k8s/configmaps?namespace=${encodeURIComponent(namespace || '')}&usage=false`, options)
}
export function getSecretsForNamespace(namespace, options) {
  return get(`/k8s/secrets?namespace=${encodeURIComponent(namespace || '')}&usage=false`, options)
}
export function getNamespaceNames(options) { return get('/k8s/namespace-names', options) }
export function getConfigMap(namespace, name, options) {
  return get(`/k8s/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
}
export function createConfigMap(payload, options) { return post('/k8s/configmaps', payload, options) }
export function updateConfigMap(namespace, name, payload, options) {
  return put(`/k8s/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, payload, options)
}
export function deleteConfigMap(namespace, name, options) {
  return remove(`/k8s/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
}
export function createSecret(payload, options) { return post('/k8s/secrets', payload, options) }
export function updateSecret(namespace, name, payload, options) {
  return put(`/k8s/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, payload, options)
}
export function deleteSecret(namespace, name, options) {
  return remove(`/k8s/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
}

export function getResourceInventory(namespace = '', options) {
  return Promise.all([
    getResourcePods(namespace, options),
    getResourceServices(namespace, options),
    getResourceDeployments(namespace, options),
  ])
}

export function getResourceService(namespace, name, options) {
  return api.get(`/k8s/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
}

export function getServiceDiscovery(namespace = '', options) {
  return api.get(`/k8s/services${namespaceQuery(namespace)}`, options)
}

export function getServiceEndpoints(namespace, name, options) {
  return api.get(`/k8s/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/endpoints`, options)
}
