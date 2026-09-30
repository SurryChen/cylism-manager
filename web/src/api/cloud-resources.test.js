import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createCloudConnection, deleteCloudDNSRecord, deleteCloudObject, downloadCloudObjectURL, getCloudConnectionPermissions, getCloudContainers, getCloudConnections, getCloudDNSRecords, getCloudObjects, getCloudProviders, updateCloudContainer } from './cloud-resources.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn(), upload: vi.fn() } }))

describe('Cloud resources api', () => {
  afterEach(() => vi.clearAllMocks())

  it('uses named resource endpoints and preserves confirmation payloads', () => {
    const options = { signal: new AbortController().signal }
    getCloudConnections(options)
    getCloudProviders(options)
    getCloudConnectionPermissions(7, options)
    getCloudDNSRecords(7, 'example.com', options)
    deleteCloudDNSRecord(7, 'record/1', { confirm: true }, options)
    getCloudContainers(7, options)
    updateCloudContainer(7, 'files/main', { acl: 'private' }, options)
    createCloudConnection({ name: '生产' }, options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections', options)
    expect(api.get).toHaveBeenCalledWith('/cloud/providers', options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/permissions', options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/dns/records?zone=example.com', options)
    expect(api.delete).toHaveBeenCalledWith('/cloud/connections/7/dns/records/record%2F1', { confirm: true }, options)
    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers', options)
    expect(api.put).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers/files%2Fmain', { acl: 'private' }, options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/cloud/connections', { name: '生产' }, options)
  })

  it('routes bucket operations using the bucket region', () => {
    getCloudObjects(7, 'files/main', { prefix: 'backups/', region: 'cn-shanghai' })
    deleteCloudObject(7, 'files/main', { key: 'x', confirm: true }, 'cn-shanghai')

    expect(api.get).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers/files%2Fmain/objects?prefix=backups%2F&region=cn-shanghai', undefined)
    expect(api.delete).toHaveBeenCalledWith('/cloud/connections/7/object-storage/containers/files%2Fmain/object?region=cn-shanghai', { key: 'x', confirm: true }, undefined)
    expect(downloadCloudObjectURL(7, 'files/main', 'a/b', 'cn-shanghai')).toBe('/api/cloud/connections/7/object-storage/containers/files%2Fmain/object?key=a%2Fb&region=cn-shanghai')
  })
})
