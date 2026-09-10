import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createCertificate, deleteCertificate, getCertificateResources, getCertificateStatus, installCertificateManager, updateIssuer } from './certificates.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('certificates api', () => {
  afterEach(() => vi.clearAllMocks())

  it('forwards one abort signal to all certificate reads', () => {
    const options = { signal: new AbortController().signal }
    getCertificateResources(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/certs', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/certs/issuers', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/certs/dns-credentials', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/certs/dns-providers', options)
  })

  it('loads cert-manager status through a named function', () => {
    const options = { signal: new AbortController().signal }
    getCertificateStatus(options)
    expect(api.get).toHaveBeenCalledWith('/certs/status', options)
  })

  it('keeps certificate mutation payloads separate from request options', () => {
    const options = { signal: new AbortController().signal }
    const body = { name: 'cert' }
    createCertificate(body, options)
    updateIssuer('Issuer', 'prod', 'issuer', body, options)
    deleteCertificate('prod', 'cert', options)
    installCertificateManager(options)
    expect(api.post).toHaveBeenCalledWith('/certs', body, options)
    expect(api.put).toHaveBeenCalledWith('/certs/issuers/Issuer/prod/issuer', body, options)
    expect(api.delete).toHaveBeenCalledWith('/certs/prod/cert', undefined, options)
    expect(api.post).toHaveBeenCalledWith('/certs/install', undefined, options)
  })
})
