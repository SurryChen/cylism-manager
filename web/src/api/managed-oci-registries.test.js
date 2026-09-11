import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createManagedRegistry, deleteManagedRegistry, getManagedRegistryCertificates, getManagedRegistryResources, repairManagedRegistry, updateManagedRegistry } from './managed-oci-registries.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('managed OCI registry api', () => {
  afterEach(() => vi.clearAllMocks())

  it('forwards one signal to every managed registry read', () => {
    const options = { signal: new AbortController().signal }
    getManagedRegistryResources(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/managed-oci-registries', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/managed-oci-registries/storage-preflight', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/managed-oci-registries/pvcs', options)
  })

  it('preserves mutation methods, paths, payloads, and options', () => {
    const options = { signal: new AbortController().signal }
    const body = { name: 'registry' }
    createManagedRegistry(body, options)
    updateManagedRegistry(7, body, options)
    repairManagedRegistry(7, options)
    deleteManagedRegistry(7, { confirm: true }, options)
    getManagedRegistryCertificates('cylism-system', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/managed-oci-registries', body, options)
    expect(api.put).toHaveBeenCalledWith('/managed-oci-registries/7', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/managed-oci-registries/7/repair', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/managed-oci-registries/7', { confirm: true }, options)
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/certificates?namespace=cylism-system', options)
  })
})
