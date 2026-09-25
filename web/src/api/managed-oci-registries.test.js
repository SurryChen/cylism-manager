import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createManagedRegistry, deleteManagedRegistry, deleteManagedRegistryRepository, deleteManagedRegistryTag, getManagedRegistryCatalog, getManagedRegistryCatalogTags, getManagedRegistryCertificates, getManagedRegistryResources, preflightManagedRegistryRepositoryDelete, preflightManagedRegistryTagDelete, refreshManagedRegistryStatus, repairManagedRegistry, updateManagedRegistry } from './managed-oci-registries.js'

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
    refreshManagedRegistryStatus(7, options)
    deleteManagedRegistry(7, { confirm: true }, options)
    getManagedRegistryCertificates('cylism-system', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/managed-oci-registries', body, options)
    expect(api.put).toHaveBeenCalledWith('/managed-oci-registries/7', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/managed-oci-registries/7/repair', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/managed-oci-registries/7/refresh-status', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/managed-oci-registries/7', { confirm: true }, options)
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/certificates?namespace=cylism-system', options)
  })

  it('uses the managed catalog endpoints without exposing registry credentials', () => {
    const options = { signal: new AbortController().signal }
    const target = { repository: 'team/orders', tag: 'v1', digest: 'sha256:abc', affected_tags: ['v1'], confirm: true }
    getManagedRegistryCatalog(7, 'opaque next', options)
    getManagedRegistryCatalogTags(7, 'team/orders', '', options)
    preflightManagedRegistryTagDelete(7, target, options)
    deleteManagedRegistryTag(7, target, options)
    preflightManagedRegistryRepositoryDelete(7, target, options)
    deleteManagedRegistryRepository(7, target, options)
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/7/catalog?cursor=opaque%20next', options)
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/7/catalog/tags?repository=team%2Forders', options)
    expect(api.post).toHaveBeenCalledWith('/managed-oci-registries/7/catalog/tags/preflight-delete', target, options)
    expect(api.delete).toHaveBeenCalledWith('/managed-oci-registries/7/catalog/tags', target, options)
    expect(api.post).toHaveBeenCalledWith('/managed-oci-registries/7/catalog/repositories/preflight-delete', target, options)
    expect(api.delete).toHaveBeenCalledWith('/managed-oci-registries/7/catalog/repositories', target, options)
  })
})
