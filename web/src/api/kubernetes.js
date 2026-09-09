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

export function getServiceDiscovery(options) {
  return api.get('/k8s/services', options)
}

export function getServiceEndpoints(namespace, name, options) {
  return api.get(`/k8s/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/endpoints`, options)
}
