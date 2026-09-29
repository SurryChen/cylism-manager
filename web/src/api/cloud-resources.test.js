import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createCloudConnection, deleteCloudDNSRecord, getCloudConnectionPermissions, getCloudContainers, getCloudConnections, getCloudDNSRecords, getCloudProviders, updateCloudContainer, validateCloudConnection } from './cloud-resources.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn(), upload: vi.fn() } }))

describe('Cloud resources api', () => {
  afterEach(() => vi.clearAllMocks())

  it('uses named resource endpoints and preserves confirmation payloads', () => {
    const options = { signal: new AbortController().signal }
    getCloudConnections(options)
    getCloudProviders(options)
    getCloudConnectionPermissions(7, options)
    validateCloudConnection(7, options)
    getCloudDNSRecords(7, 'example.com', options)
    deleteCloudDNSRecord(7, 'record/1', { confirm: true }, options)
    getCloudContainers(7, options)
    updateCloudContainer(7, 'files/main', { acl: 'private' }, options)
    createCloudConnection({ name: '生产' }, options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections', options)
    expect(api.get).toHaveBeenCalledWith('/cloud/providers', options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/permissions', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/cloud/connections/7/validate', undefined, options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/dns/records?zone=example.com', options)
    expect(api.delete).toHaveBeenCalledWith('/cloud/connections/7/dns/records/record%2F1', { confirm: true }, options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers', options)
    expect(api.put).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers/files%2Fmain', { acl: 'private' }, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/cloud/connections', { name: '生产' }, options)
  })
})
