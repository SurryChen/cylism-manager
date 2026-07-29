import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Domains from './Domains.vue'
import { api } from '../api/index.js'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  api.get.mockImplementation(path => {
    if (path === '/domains') return Promise.resolve([{ id: 1, hostname: 'api.example.com', namespace: 'production', issuer_ref: 'letsencrypt-prod', tls_secret_name: 'cylism-domain-1-tls', certificate_name: 'cylism-domain-1', certificate: { status: 'Ready', renewal_time: '2026-10-01T00:00:00Z' }, application_count: 0 }])
    if (path === '/k8s/namespaces') return Promise.resolve([{ name: 'production', status: 'Active' }])
    if (path === '/certs/issuers') return Promise.resolve([{ name: 'letsencrypt-prod', kind: 'ClusterIssuer', ready: true }])
    return Promise.resolve([])
  })
})

describe('Domains view', () => {
  it('renders managed certificate state', async () => {
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await nextTick()
    expect(wrapper.text()).toContain('api.example.com')
    expect(wrapper.text()).toContain('已就绪')
    expect(wrapper.text()).toContain('cylism-domain-1-tls')
  })

  it('loads namespaces and ready ClusterIssuers before creating a domain', async () => {
    const wrapper = mount(Domains)
    await wrapper.get('.btn-primary').trigger('click')
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/k8s/namespaces')
    expect(api.get).toHaveBeenCalledWith('/certs/issuers')
    expect(wrapper.text()).toContain('申请 HTTPS 域名')
    expect(wrapper.text()).toContain('letsencrypt-prod')
  })
})
