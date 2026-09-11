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

function claimPath(name) {
  return `/k8s/persistent-volume-claims/${encodeURIComponent(name)}`
}

function post(path, body, options) {
  if (options === undefined && body === undefined) return api.post(path)
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

function remove(path, body, options) {
  return options === undefined ? api.delete(path, body) : api.delete(path, body, options)
}

export function createNamespace(body, options) {
  return post('/k8s/namespaces', body, options)
}

export function createPersistentVolumeClaim(body, options) {
  return post('/k8s/persistent-volume-claims', body, options)
}

export function createPersistentVolumeMigration(name, body, options) {
  return post(`${claimPath(name)}/migrations`, body, options)
}

export function cleanupPersistentVolumeMigration(migrationID, options) {
  return post(`/k8s/persistent-volume-migrations/${encodeURIComponent(migrationID)}/cleanup`, undefined, options)
}

export function createPersistentVolumeBackup(name, body, options) {
  return post(`${claimPath(name)}/backups`, body, options)
}

export function restorePersistentVolumeBackup(name, backupID, body, options) {
  return post(`${claimPath(name)}/backups/${encodeURIComponent(backupID)}/restore`, body, options)
}

export function createPersistentVolumeImport(name, body, options) {
  return post(`${claimPath(name)}/imports`, body, options)
}

export function deletePersistentVolumeImportBackup(name, importID, body, options) {
  return remove(`${claimPath(name)}/imports/${encodeURIComponent(importID)}/backup`, body, options)
}

export function deletePersistentVolumeClaim(name, body, options) {
  return remove(claimPath(name), body, options)
}
