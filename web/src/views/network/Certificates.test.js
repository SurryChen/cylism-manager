import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Certificates from './Certificates.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

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
	vi.clearAllMocks()
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return Promise.resolve({ state: 'ready', message: 'cert-manager 已就绪' })
      if (path === '/certs/issuers') return Promise.resolve(issuers)
      if (path === '/certs/dns-credentials') return Promise.resolve([])
      if (path === '/certs/dns-providers') return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [], webhook: true, status: { state: 'not_installed', ready: false, message: 'AliDNS Webhook 尚未安装' } }])
      return Promise.resolve([{ name: 'api-example', namespace: 'production', domains: ['api.example.com'], issuer: 'letsencrypt-dns', issuer_kind: 'ClusterIssuer', secret_name: 'api-example-tls', expiry_date: '2026-10-01T12:00:00Z', status: 'Ready' }])
    })
    api.post.mockResolvedValue({})
    api.delete.mockResolvedValue({})
  })

  it('loads only certificate inventory on mount', async () => {
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).toContain('api-example-tls')
    expect(api.get).toHaveBeenCalledWith('/certs/status', expect.anything())
    expect(api.get).toHaveBeenCalledWith('/certs', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/certs/issuers', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/certs/dns-credentials', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/certs/dns-providers', expect.anything())
  })

  it('loads issuance configuration only after it is opened', async () => {
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).not.toContain('broken-issuer')
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('letsencrypt-dns')
    expect(wrapper.text()).toContain('broken-issuer')
    expect(wrapper.text()).toContain('不可用')
    expect(api.get).toHaveBeenCalledWith('/certs/issuers', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/certs/dns-credentials', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/certs/dns-providers', expect.anything())
    await wrapper.get('[role="tab"]:nth-child(3)').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('阿里云 DNS')
    expect(api.get).toHaveBeenCalledWith('/certs/dns-providers', expect.anything())
  })

  it('loads issuers when the certificate form is opened', async () => {
    const wrapper = mount(Certificates)
    await settle()

    await wrapper.get('[data-testid="add-certificate"]').trigger('click')
    await settle()

    expect(api.get).toHaveBeenCalledWith('/certs/issuers', expect.anything())
  })

  it('creates a certificate with the selected issuer reference and kind', async () => {
    const wrapper = mount(Certificates)
    await settle()
    await wrapper.get('[data-testid="add-certificate"]').trigger('click')
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

  it('opens a prefilled certificate form from the Registry link', async () => {
    window.location.hash = '#/network?tab=certificates&create=1&namespace=cylism-system&domains=registry.internal'
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.get('input[placeholder="default"]').element.value).toBe('cylism-system')
    expect(wrapper.get('input[placeholder="example.com,*.example.com"]').element.value).toBe('registry.internal')
    wrapper.unmount()
    window.location.hash = ''
  })

  it('shows setup status without loading certificate resources when cert-manager is absent', async () => {
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return Promise.resolve({ state: 'not_installed', message: '未检测到完整的 cert-manager CRD', installer_available: true })
      return Promise.reject(new Error(`unexpected request: ${path}`))
    })
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).toContain('cert-manager 未安装')
    expect(wrapper.text()).toContain('安装 cert-manager')
    expect(api.get).not.toHaveBeenCalledWith('/certs')
    expect(api.get).not.toHaveBeenCalledWith('/certs/issuers')
  })

  it('uses the certificate system icon for an empty ready state', async () => {
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return Promise.resolve({ state: 'ready', message: 'cert-manager 已就绪' })
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.get('[data-testid="certificate-empty-icon"]').element.tagName).toBe('svg')
    expect(wrapper.text()).not.toContain('🔒')
  })

  it('shows a local error when the initial certificate read fails', async () => {
    api.get.mockRejectedValueOnce(new Error('cert-manager unavailable'))
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).toContain('cert-manager unavailable')
  })
})
