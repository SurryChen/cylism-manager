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
	getPersistentVolumeReferences,
  getPersistentVolumeUsage,
  restorePersistentVolumeBackup,
} from './storage.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

describe('storage api', () => {
  it('loads a paginated persistent volume inventory with one signal', () => {
    const options = { signal: new AbortController().signal }
    getPersistentVolumeInventory({ page: 2, size: 20, namespace: 'default' }, options)
    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims?page=2&size=20&namespace=default', options)
  })

  it('loads usage only for the displayed persistent volumes', () => {
    const options = { signal: new AbortController().signal }
    getPersistentVolumeUsage([{ namespace: 'default', name: 'data' }], options)
    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims/usage?claim=default%2Fdata', options)
  })

  it('loads references only for the displayed persistent volumes', () => {
    const options = { signal: new AbortController().signal }
    getPersistentVolumeReferences([{ namespace: 'default', name: 'data' }], options)
    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims/references?claim=default%2Fdata', options)
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
