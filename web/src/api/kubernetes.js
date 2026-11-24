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
export function getSecretMetadataPage(namespace, { limit = 50, continueToken = '', ...options } = {}) {
  const query = new URLSearchParams({ namespace: namespace || '', metadata: 'true', limit: String(limit) })
  if (continueToken) query.set('continue', continueToken)
  return get(`/k8s/secrets?${query.toString()}`, options)
}
export function getNamespaceNames(options) { return get('/k8s/namespace-names', options) }
export function getConfigMap(namespace, name, options) {
  return get(`/k8s/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
}
export function getSecret(namespace, name, options) {
  return get(`/k8s/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, options)
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

export function getLightweightServiceDiscovery(namespace = '', options) {
  const query = namespace
    ? `?namespace=${encodeURIComponent(namespace)}&endpoint_count=false`
    : '?endpoint_count=false'
  return get(`/k8s/services${query}`, options)
}

export function getServiceEndpoints(namespace, name, options) {
  return api.get(`/k8s/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/endpoints`, options)
}

// Resource pages use a cluster-bound API so no request can silently fall back
// to the platform's local Kubernetes client.
export function createClusterKubernetesApi(clusterID) {
  if (!/^[1-9]\d*$/.test(String(clusterID))) throw new Error('集群 ID 无效')
  const base = `/clusters/${clusterID}/k8s`
  const path = (resource, namespace, name) => `${base}/${resource}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
  return {
    getWorkloadDeployments: options => api.get(`${base}/deployments`, options),
    getWorkloadStatefulSets: options => api.get(`${base}/statefulsets`, options),
    getWorkloadDaemonSets: options => api.get(`${base}/daemonsets`, options),
    getWorkloadPods: options => api.get(`${base}/pods`, options),
    getWorkloadServers: async options => (await api.get('/servers', options)).filter(server => Number(server.cluster_id) === Number(clusterID)),
    getWorkloadDeploymentPods: (namespace, name, options) => api.get(`${path('deployments', namespace, name)}/pods`, options),
    getWorkloadDeploymentRevisions: (namespace, name, options) => api.get(`${path('deployments', namespace, name)}/revisions`, options),
    scaleWorkload: (kind, namespace, name, replicas) => api.patch(`${path(kind === 'statefulset' ? 'statefulsets' : 'deployments', namespace, name)}/scale`, { replicas }),
    updateWorkloadImage: (namespace, name, payload) => api.patch(`${path('deployments', namespace, name)}/image`, payload),
    rollbackWorkload: (namespace, name, revision) => api.post(`${path('deployments', namespace, name)}/rollback`, { revision }),
    getLightweightServiceDiscovery: (namespace = '', options) => api.get(`${base}/services${namespace ? `?namespace=${encodeURIComponent(namespace)}&endpoint_count=false` : '?endpoint_count=false'}`, options),
    getServiceEndpoints: (namespace, name, options) => api.get(`${path('services', namespace, name)}/endpoints`, options),
    getConfigMapsForNamespace: (namespace, options) => api.get(`${base}/configmaps?namespace=${encodeURIComponent(namespace || '')}&usage=false`, options),
    getSecretsForNamespace: (namespace, options) => api.get(`${base}/secrets?namespace=${encodeURIComponent(namespace || '')}&usage=false`, options),
    getSecretMetadataPage: (namespace, { limit = 50, continueToken = '', ...options } = {}) => {
      const query = new URLSearchParams({ namespace: namespace || '', metadata: 'true', limit: String(limit) })
      if (continueToken) query.set('continue', continueToken)
      return api.get(`${base}/secrets?${query}`, options)
    },
    getNamespaceNames: options => api.get(`${base}/namespace-names`, options),
    getConfigMap: (namespace, name, options) => api.get(path('configmaps', namespace, name), options),
    getSecret: (namespace, name, options) => api.get(path('secrets', namespace, name), options),
    createConfigMap: (payload, options) => api.post(`${base}/configmaps`, payload, options),
    updateConfigMap: (namespace, name, payload, options) => api.put(path('configmaps', namespace, name), payload, options),
    deleteConfigMap: (namespace, name, options) => api.delete(path('configmaps', namespace, name), undefined, options),
    createSecret: (payload, options) => api.post(`${base}/secrets`, payload, options),
    updateSecret: (namespace, name, payload, options) => api.put(path('secrets', namespace, name), payload, options),
    deleteSecret: (namespace, name, options) => api.delete(path('secrets', namespace, name), undefined, options),
  }
}
