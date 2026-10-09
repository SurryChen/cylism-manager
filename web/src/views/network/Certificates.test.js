import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Certificates from './Certificates.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
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
    api.put.mockResolvedValue({})
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

  it('starts the certificate request before the status check completes', async () => {
    let resolveStatus
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return new Promise(resolve => { resolveStatus = resolve })
      if (path === '/certs') return Promise.resolve([])
      return Promise.resolve([])
    })

    const wrapper = mount(Certificates)

    expect(api.get).toHaveBeenCalledWith('/certs/status', expect.anything())
    expect(api.get).toHaveBeenCalledWith('/certs', expect.anything())

    resolveStatus({ state: 'ready', message: 'cert-manager 已就绪' })
    await settle()
    wrapper.unmount()
  })

  it('opens with an issuer list and switches to separate create and advanced views', async () => {
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).not.toContain('broken-issuer')
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('证书申请配置')
    expect(wrapper.text()).toContain('broken-issuer')
    expect(wrapper.find('.certificate-setup').exists()).toBe(false)
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
    expect(api.get).toHaveBeenCalledWith('/certs/issuers', expect.anything())
    expect(api.get).toHaveBeenCalledWith('/certs/dns-credentials', expect.anything())
    expect(api.get).toHaveBeenCalledWith('/certs/dns-providers', expect.anything())
    await wrapper.get('[data-testid="add-issuer-setup"]').trigger('click')
    expect(wrapper.text()).toContain('阿里云 DNS')
    expect(wrapper.find('.certificate-setup').exists()).toBe(true)
    expect(wrapper.find('.setup-existing').exists()).toBe(false)
    await wrapper.get('.certificate-setup + .advanced-toggle').trigger('click')
    expect(wrapper.find('.certificate-setup').exists()).toBe(false)
    await wrapper.get('.advanced-toggle').trigger('click')
    expect(wrapper.text()).toContain('broken-issuer')
    expect(wrapper.text()).toContain('不可用')
    expect(wrapper.find('.setup-existing').exists()).toBe(false)
    await wrapper.get('[role="tab"]:nth-child(3)').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('AliDNS Webhook 尚未安装')
    await wrapper.get('.advanced-back').trigger('click')
    expect(wrapper.find('.setup-existing').exists()).toBe(true)
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
  })

  it('creates a provider account and managed issuer for the selected cluster', async () => {
    api.get.mockImplementation(path => {
      if (path.endsWith('/certs/status')) return Promise.resolve({ state: 'ready' })
      if (path.endsWith('/certs/issuers') || path.endsWith('/certs/dns-credentials') || path.endsWith('/certs')) return Promise.resolve([])
      if (path.endsWith('/certs/dns-providers')) return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [{ key: 'access_key_id', label: 'AccessKey ID', required: true }, { key: 'access_key_secret', label: 'AccessKey Secret', required: true, secret: true }], status: { ready: true } }])
    })
    api.post.mockImplementation(path => Promise.resolve(path.endsWith('/dns-credentials') ? { id: 42 } : {}))
    const wrapper = mount(Certificates, { props: { clusterId: 2 } })
    await settle()
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()
    await wrapper.get('[data-testid="add-issuer-setup"]').trigger('click')
    await wrapper.get('#setup-access_key_id').setValue('key-id')
    await wrapper.get('#setup-access_key_secret').setValue('key-secret')
    await wrapper.get('#setup-email').setValue('ops@example.com')
    await wrapper.get('.certificate-setup').trigger('submit')
    await settle()

    expect(api.post).toHaveBeenNthCalledWith(1, '/clusters/2/k8s/certs/dns-credentials', expect.objectContaining({ provider: 'alidns', namespace: 'cert-manager', values: { access_key_id: 'key-id', access_key_secret: 'key-secret' } }))
    expect(api.post).toHaveBeenNthCalledWith(2, '/clusters/2/k8s/certs/issuers', expect.objectContaining({ name: 'cylism-alidns-certificates', kind: 'ClusterIssuer', mode: 'acme_dns01', credential_id: 42, email: 'ops@example.com' }))
    expect(api.put).not.toHaveBeenCalled()
  })

  it('reuses the linked account when an existing issuer is selected', async () => {
    api.get.mockImplementation(path => {
      if (path.endsWith('/certs/status')) return Promise.resolve({ state: 'ready' })
      if (path.endsWith('/certs/issuers')) return Promise.resolve([
        { name: 'external-issuer', kind: 'ClusterIssuer', mode: 'acme_dns01', dns_provider: 'alidns', credential_secret_name: 'external', email: 'other@example.com' },
        { name: 'cylism-alidns-certificates', kind: 'ClusterIssuer', mode: 'acme_dns01', dns_provider: 'alidns', credential_secret_name: 'cylism-dns-alidns-7', email: 'ops@example.com', ready: true },
      ])
      if (path.endsWith('/certs/dns-credentials')) return Promise.resolve([{ id: 7, name: 'production-account', provider: 'alidns', namespace: 'cert-manager', secret_name: 'cylism-dns-alidns-7', enabled: true }])
      if (path.endsWith('/certs/dns-providers')) return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [], status: { ready: true } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates, { props: { clusterId: 2 } })
    await settle()
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()
    expect(wrapper.text()).toContain('external-issuer')
    await wrapper.findAll('.setup-table tbody tr')[1].get('button').trigger('click')
    expect(wrapper.get('#setup-email').element.value).toBe('ops@example.com')
    expect(wrapper.get('.certificate-setup select[aria-label="验证账号"]').element.value).toBe('7')
    await wrapper.get('#setup-email').setValue('new@example.com')
    await wrapper.get('.certificate-setup').trigger('submit')
    await settle()

    expect(api.post).not.toHaveBeenCalled()
    expect(api.put).toHaveBeenCalledWith('/clusters/2/k8s/certs/issuers/ClusterIssuer/_/cylism-alidns-certificates', expect.objectContaining({ credential_id: 7, email: 'new@example.com' }))
  })

  it('recognizes an existing DNS issuer even when its name was not generated by the platform', async () => {
    api.get.mockImplementation(path => {
      if (path.endsWith('/certs/status')) return Promise.resolve({ state: 'ready' })
      if (path.endsWith('/certs/issuers')) return Promise.resolve([{ name: 'letsencrypt-prod', kind: 'ClusterIssuer', mode: 'acme_dns01', dns_provider: 'alidns', credential_secret_name: 'cylism-dns-alidns-9', email: 'old@example.com', ready: true }])
      if (path.endsWith('/certs/dns-credentials')) return Promise.resolve([{ id: 9, name: 'ali-account', provider: 'alidns', namespace: 'cert-manager', secret_name: 'cylism-dns-alidns-9', enabled: true }])
      if (path.endsWith('/certs/dns-providers')) return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [], status: { ready: true } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates, { props: { clusterId: 2 } })
    await settle()
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()

    expect(wrapper.text()).toContain('letsencrypt-prod')
    await wrapper.get('.setup-table tbody tr button').trigger('click')
    expect(wrapper.get('#setup-email').element.value).toBe('old@example.com')
    expect(wrapper.get('.certificate-setup select[aria-label="验证账号"]').element.value).toBe('9')
    await wrapper.get('#setup-email').setValue('new@example.com')
    await wrapper.get('.certificate-setup').trigger('submit')
    await settle()

    expect(api.post).not.toHaveBeenCalled()
    expect(api.put).toHaveBeenCalledWith('/clusters/2/k8s/certs/issuers/ClusterIssuer/_/letsencrypt-prod', expect.objectContaining({ credential_id: 9, email: 'new@example.com' }))
  })

  it('confirms deletion from the list and keeps component issuers read-only', async () => {
    api.get.mockImplementation(path => {
      if (path.endsWith('/certs/status')) return Promise.resolve({ state: 'ready' })
      if (path.endsWith('/certs/issuers')) return Promise.resolve([
        { name: 'cylism-alidns-webhook-selfsign', kind: 'Issuer', namespace: 'cert-manager', mode: 'self_signed', ready: true },
        { name: 'letsencrypt-prod', kind: 'ClusterIssuer', mode: 'acme_dns01', dns_provider: 'alidns', ready: true },
      ])
      if (path.endsWith('/certs/dns-providers')) return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [], status: { ready: true } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates, { props: { clusterId: 2 } })
    await settle()
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()

    expect(wrapper.findAll('.setup-table tbody tr')[0].text()).toContain('组件管理')
    expect(wrapper.findAll('.setup-table tbody tr')[0].find('button').exists()).toBe(false)
    await wrapper.findAll('.setup-table tbody tr')[1].findAll('button')[1].trigger('click')
    expect(api.delete).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await settle()
    expect(api.delete).toHaveBeenCalledWith('/clusters/2/k8s/certs/issuers/ClusterIssuer/_/letsencrypt-prod')
  })

  it('shows a read error instead of an empty issuer list when configuration loading fails', async () => {
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return Promise.resolve({ state: 'ready' })
      if (path === '/certs/issuers') return Promise.reject(new Error('读取签发者失败'))
      if (path === '/certs/dns-providers') return Promise.resolve([{ id: 'alidns', name: '阿里云 DNS', fields: [], status: { ready: true } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates)
    await settle()
    await wrapper.get('[data-testid="open-issuance-config"]').trigger('click')
    await settle()

    expect(wrapper.get('.setup-load-error').text()).toContain('读取签发者失败')
    expect(wrapper.find('.setup-existing').exists()).toBe(false)
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

  it('shows setup status when cert-manager is absent even if the parallel certificate request fails', async () => {
    api.get.mockImplementation(path => {
      if (path === '/certs/status') return Promise.resolve({ state: 'not_installed', message: '未检测到完整的 cert-manager CRD', installer_available: true })
      if (path === '/certs') return Promise.reject(new Error('cert-manager resource is unavailable'))
      return Promise.resolve([])
    })
    const wrapper = mount(Certificates)
    await settle()

    expect(wrapper.text()).toContain('cert-manager 未安装')
    expect(wrapper.text()).toContain('安装 cert-manager')
    expect(wrapper.text()).not.toContain('cert-manager resource is unavailable')
    expect(api.get).toHaveBeenCalledWith('/certs', expect.anything())
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
