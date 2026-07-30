import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { reactive } from 'vue'
import { mount } from '@vue/test-utils'
import Domains from './Domains.vue'
import { api } from '../api/index.js'

const push = vi.fn()
const route = reactive({ query: { project_id: '1', environment_id: '2' } })
vi.mock('vue-router', () => ({ useRouter: () => ({ push }), useRoute: () => route }))
vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  api.get.mockImplementation(path => {
    if (path === '/domains?environment_id=2') return Promise.resolve([{ id: 1, environment_id: 2, hostname: 'api.example.com', namespace: 'production', issuer_ref: 'letsencrypt-prod', tls_secret_name: 'cylism-domain-1-tls', certificate_name: 'cylism-domain-1', certificate: { status: 'Ready', renewal_time: '2026-10-01T00:00:00Z' }, application_count: 0 }])
    if (path === '/projects') return Promise.resolve([{ id: 1, environments: [{ id: 2, name: 'production', namespace: 'production' }] }])
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

  it('loads ready ClusterIssuers before creating a domain in the selected environment', async () => {
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.get('.btn-primary').trigger('click')
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/certs/issuers')
    expect(wrapper.text()).toContain('申请 HTTPS 域名')
    expect(wrapper.text()).toContain('letsencrypt-prod')
  })
})
