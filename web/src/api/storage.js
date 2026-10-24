import { api } from './index.js'

function clusterStoragePath(clusterID, path) {
  return `/clusters/${encodeURIComponent(clusterID)}/k8s${path}`
}

export function getClusterPersistentVolumeInventory(clusterID, params = {}, options) {
  return api.get(queryPath(clusterStoragePath(clusterID, '/persistent-volume-claims'), params), options)
}
export function getClusterPersistentVolumeReferences(clusterID, claims, options) {
  const query = new URLSearchParams()
  claims.forEach(claim => query.append('claim', `${claim.namespace}/${claim.name}`))
  return api.get(`${clusterStoragePath(clusterID, '/persistent-volume-claims/references')}?${query}`, options)
}
export function getClusterPersistentVolumeUsage(clusterID, claims, options) {
  const query = new URLSearchParams()
  claims.forEach(claim => query.append('claim', `${claim.namespace}/${claim.name}`))
  return api.get(`${clusterStoragePath(clusterID, '/persistent-volume-claims/usage')}?${query}`, options)
}
export function getClusterStorageClasses(clusterID, options) { return api.get(clusterStoragePath(clusterID, '/storage-classes'), options) }
export function getClusterStorageNamespaces(clusterID, options) { return api.get(clusterStoragePath(clusterID, '/namespace-names'), options) }
export function createClusterStorageNamespace(clusterID, name) { return api.post(clusterStoragePath(clusterID, '/namespaces'), { name }) }
export function createClusterPersistentVolumeClaim(clusterID, body) { return api.post(clusterStoragePath(clusterID, '/persistent-volume-claims'), body) }
export function adoptClusterPersistentVolumeClaim(clusterID, name, body) { return api.post(clusterStoragePath(clusterID, `/persistent-volume-claims/${encodeURIComponent(name)}/adopt`), body) }
export function deleteClusterPersistentVolumeClaim(clusterID, name, body) { return api.delete(clusterStoragePath(clusterID, `/persistent-volume-claims/${encodeURIComponent(name)}`), body) }

function queryPath(path, params = {}) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value))
  })
  const value = query.toString()
  return value ? `${path}?${value}` : path
}

// The one-argument form remains available for existing list consumers.
export function getPersistentVolumeInventory(params = {}, options) {
  if (params?.signal && options === undefined) return api.get('/k8s/persistent-volume-claims', params)
  return api.get(queryPath('/k8s/persistent-volume-claims', params), options)
}

export function getPersistentVolumeUsage(claims = [], options) {
  if (claims?.signal && options === undefined) return api.get('/k8s/persistent-volume-claims/usage', claims)
  const query = new URLSearchParams()
  claims.forEach(claim => query.append('claim', `${claim.namespace}/${claim.name}`))
  const suffix = query.toString()
  return api.get(`/k8s/persistent-volume-claims/usage${suffix ? `?${suffix}` : ''}`, options)
}

export function getPersistentVolumeReferences(claims = [], options) {
  const query = new URLSearchParams()
  claims.forEach(claim => query.append('claim', `${claim.namespace}/${claim.name}`))
  const suffix = query.toString()
  return api.get(`/k8s/persistent-volume-claims/references${suffix ? `?${suffix}` : ''}`, options)
}
export function getPersistentVolumeStorageClasses(options) { return api.get('/k8s/storage-classes', options) }
export function getPersistentVolumeMigrations(options) { return api.get('/k8s/persistent-volume-migrations', options) }
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

export function adoptPersistentVolumeClaim(name, body, options) {
  return post(`${claimPath(name)}/adopt`, body, options)
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
