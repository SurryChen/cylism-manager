import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  cleanupPersistentVolumeMigration,
  createNamespace,
  createPersistentVolumeBackup,
  createPersistentVolumeClaim,
  createPersistentVolumeImport,
  createPersistentVolumeMigration,
  deletePersistentVolumeClaim,
  deletePersistentVolumeImportBackup,
  getPersistentVolumeInventory,
  getPersistentVolumeUsage,
  restorePersistentVolumeBackup,
} from './storage.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

describe('storage api', () => {
  it('loads the complete persistent volume inventory with one signal', () => {
    const options = { signal: new AbortController().signal }
    getPersistentVolumeInventory(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/k8s/persistent-volume-claims', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/k8s/storage-classes', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/k8s/persistent-volume-migrations', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/nodes', options)
    expect(api.get).toHaveBeenNthCalledWith(5, '/servers', options)
    expect(api.get).toHaveBeenNthCalledWith(6, '/k8s/namespace-names', options)
  })

  it('forwards options to usage requests', () => {
    const options = { signal: new AbortController().signal }
    getPersistentVolumeUsage(options)
    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims/usage', options)
  })

  it('uses the existing persistent volume mutation endpoints and forwards request options', () => {
    const name = 'data/a'
    const body = { namespace: 'default' }
    const options = { signal: new AbortController().signal }

    createNamespace(body, options)
    createPersistentVolumeClaim(body, options)
    createPersistentVolumeMigration(name, body, options)
    cleanupPersistentVolumeMigration('migration/a', options)
    createPersistentVolumeBackup(name, body, options)
    restorePersistentVolumeBackup(name, 'backup/a', body, options)
    createPersistentVolumeImport(name, body, options)
    deletePersistentVolumeImportBackup(name, 'import/a', body, options)
    deletePersistentVolumeClaim(name, body, options)

    expect(api.post).toHaveBeenNthCalledWith(1, '/k8s/namespaces', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/k8s/persistent-volume-claims', body, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/k8s/persistent-volume-claims/data%2Fa/migrations', body, options)
    expect(api.post).toHaveBeenNthCalledWith(4, '/k8s/persistent-volume-migrations/migration%2Fa/cleanup', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(5, '/k8s/persistent-volume-claims/data%2Fa/backups', body, options)
    expect(api.post).toHaveBeenNthCalledWith(6, '/k8s/persistent-volume-claims/data%2Fa/backups/backup%2Fa/restore', body, options)
    expect(api.post).toHaveBeenNthCalledWith(7, '/k8s/persistent-volume-claims/data%2Fa/imports', body, options)
    expect(api.delete).toHaveBeenNthCalledWith(1, '/k8s/persistent-volume-claims/data%2Fa/imports/import%2Fa/backup', body, options)
    expect(api.delete).toHaveBeenNthCalledWith(2, '/k8s/persistent-volume-claims/data%2Fa', body, options)
  })
})
