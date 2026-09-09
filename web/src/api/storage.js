import { api } from './index.js'

export function getPersistentVolumeInventory(options) {
  return Promise.all([
    api.get('/k8s/persistent-volume-claims', options),
    api.get('/k8s/storage-classes', options),
    api.get('/k8s/persistent-volume-migrations', options),
    api.get('/nodes', options),
    api.get('/servers', options),
    api.get('/k8s/namespace-names', options),
  ])
}

export function getPersistentVolumeUsage(options) { return api.get('/k8s/persistent-volume-claims/usage', options) }
export function getPersistentVolumeBackups(name, environmentID, options) { return api.get(`/k8s/persistent-volume-claims/${encodeURIComponent(name)}/backups?environment_id=${encodeURIComponent(environmentID)}`, options) }
export function getPersistentVolumeImports(name, environmentID, options) { return api.get(`/k8s/persistent-volume-claims/${encodeURIComponent(name)}/imports?environment_id=${encodeURIComponent(environmentID)}`, options) }
