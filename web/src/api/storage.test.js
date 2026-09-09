import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getPersistentVolumeInventory, getPersistentVolumeUsage } from './storage.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

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
})
