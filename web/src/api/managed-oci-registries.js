import { api } from './index.js'

function post(path, body, options) {
  if (body === undefined && options === undefined) return api.post(path)
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function remove(path, body, options) {
  if (options === undefined) return api.delete(path, body)
  return api.delete(path, body, options)
}

export function getManagedRegistries(options) { return api.get('/managed-oci-registries', options) }
export function getManagedRegistryStoragePreflight(options) { return api.get('/managed-oci-registries/storage-preflight', options) }
export function getManagedRegistryPVCs(options) { return api.get('/managed-oci-registries/pvcs', options) }
export function getManagedRegistryCertificates(namespace, options) { return api.get(`/managed-oci-registries/certificates?namespace=${encodeURIComponent(namespace)}`, options) }
export function createManagedRegistry(body, options) { return post('/managed-oci-registries', body, options) }
export function updateManagedRegistry(id, body, options) { return options === undefined ? api.put(`/managed-oci-registries/${encodeURIComponent(id)}`, body) : api.put(`/managed-oci-registries/${encodeURIComponent(id)}`, body, options) }
export function repairManagedRegistry(id, options) { return post(`/managed-oci-registries/${encodeURIComponent(id)}/repair`, undefined, options) }
export function deleteManagedRegistry(id, body, options) { return remove(`/managed-oci-registries/${encodeURIComponent(id)}`, body, options) }

export function getManagedRegistryResources(options) {
  return Promise.all([
    getManagedRegistries(options),
    getManagedRegistryStoragePreflight(options),
    getManagedRegistryPVCs(options),
  ])
}
