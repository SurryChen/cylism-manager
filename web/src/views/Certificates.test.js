import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Certificates from './Certificates.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

const issuers = [
  { name: 'letsencrypt-dns', kind: 'ClusterIssuer', ready: true },
  { name: 'namespace-issuer', namespace: 'production', kind: 'Issuer', ready: true },
  { name: 'broken-issuer', kind: 'ClusterIssuer', ready: false, reason: 'MissingCredentials' },
]

async function settle() {
  await new Promise(resolve => setTimeout(resolve, 0))
}

describe('Certificates view', () => {
  beforeEach(() => {
    api.get.mockImplementation(path => {
      if (path === '/certs/issuers') return Promise.resolve(issuers)
      return Promise.resolve([{ name: 'api-example', namespace: 'production', domains: ['api.example.com'], issuer: 'letsencrypt-dns', issuer_kind: 'ClusterIssuer', secret_name: 'api-example-tls', expiry_date: '2026-10-01T12:00:00Z', status: 'Ready' }])
    })
    api.post.mockResolvedValue({})
    api.delete.mockResolvedValue({})
  })

  it('shows compact filtering and issuer operations status', async () => {
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.find('.filter-bar .form-select').exists()).toBe(true)
    expect(wrapper.text()).toContain('api-example-tls')
    expect(wrapper.text()).toContain('letsencrypt-dns')
    expect(wrapper.text()).toContain('不可用')
  })

  it('creates a certificate with the selected issuer reference and kind', async () => {
    const wrapper = mount(Certificates)
    await settle()
    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="my-cert"]').setValue('shop-cert')
    await wrapper.get('input[placeholder="default"]').setValue('production')
    await wrapper.get('input[placeholder="example.com,*.example.com"]').setValue('shop.example.com,*.shop.example.com')
    await settle()
    await wrapper.get('.modal select').setValue('Issuer/production/namespace-issuer')
    await wrapper.get('form').trigger('submit')
    await settle()

    expect(api.post).toHaveBeenCalledWith('/certs', {
      name: 'shop-cert',
      namespace: 'production',
      domains: ['shop.example.com', '*.shop.example.com'],
      issuer_ref: 'namespace-issuer',
      issuer_kind: 'Issuer',
    })
  })
})
