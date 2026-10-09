import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  createManagedDomain,
  deleteManagedDomain,
  getClusterDomainOptions,
  getDomainOptions,
  getManagedDomains,
  importManagedDomainCertificate,
  retryManagedDomainCertificate,
  updateManagedDomain,
} from './domains.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('domains api', () => {
  it('builds managed domain filters', () => {
    const options = { signal: new AbortController().signal }
    getManagedDomains({ environmentID: 2, unassigned: false }, options)
    expect(api.get).toHaveBeenCalledWith('/domains?environment_id=2', options)
  })

  it('forwards options for domain options', () => {
    const options = { signal: new AbortController().signal }
    getDomainOptions(options)
    expect(api.get).toHaveBeenCalledWith('/certs/issuers', options)
    getClusterDomainOptions(7, options)
    expect(api.get).toHaveBeenCalledWith('/clusters/7/k8s/certs/issuers', options)
  })

  it('owns managed domain mutations with encoded identifiers', () => {
    const options = { signal: new AbortController().signal }
    const body = { hostname: 'api.example.com', environment_id: 2 }
    createManagedDomain(body, options)
    updateManagedDomain('domain/1', body, options)
    importManagedDomainCertificate({ certificate_name: 'existing' }, options)
    retryManagedDomainCertificate('domain/1', options)
    deleteManagedDomain('domain/1', options)

    expect(api.post).toHaveBeenNthCalledWith(1, '/domains', body, options)
    expect(api.put).toHaveBeenCalledWith('/domains/domain%2F1', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/domains/import', { certificate_name: 'existing' }, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/domains/domain%2F1/certificate', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/domains/domain%2F1', undefined, options)
  })
})
