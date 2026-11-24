import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), remove: vi.fn() }))
vi.mock('./index.js', () => ({ api: { get, post, delete: remove } }))

import { getCertificateStatus, getDNSCredentials } from './certificates.js'
import { getIngresses } from './sites.js'
import { createClusterPersistentVolumeClaim, getClusterPersistentVolumeInventory } from './storage.js'

describe('cluster infrastructure API paths', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); remove.mockReset() })

  it('addresses network and certificate reads through the selected cluster', () => {
    getIngresses({ signal: 'test' }, 2)
    getCertificateStatus(undefined, 2)
    getDNSCredentials(undefined, 2)
    expect(get.mock.calls.map(([path]) => path)).toEqual([
      '/clusters/2/k8s/ingresses',
      '/clusters/2/k8s/certs/status',
      '/clusters/2/k8s/certs/dns-credentials',
    ])
  })

  it('keeps PVC reads and writes under the same cluster', () => {
    getClusterPersistentVolumeInventory(7, { page: 2, size: 20 })
    createClusterPersistentVolumeClaim(7, { namespace: 'default', name: 'data', storage: '5Gi' })
    expect(get).toHaveBeenCalledWith('/clusters/7/k8s/persistent-volume-claims?page=2&size=20', undefined)
    expect(post).toHaveBeenCalledWith('/clusters/7/k8s/persistent-volume-claims', { namespace: 'default', name: 'data', storage: '5Gi' })
  })
})
