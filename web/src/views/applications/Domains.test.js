import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { reactive } from 'vue'
import { mount } from '@vue/test-utils'
import Domains from './Domains.vue'
import { api } from '../../api/index.js'

const push = vi.fn()
const route = reactive({ query: { project_id: '1', environment_id: '2' } })
vi.mock('vue-router', () => ({ useRouter: () => ({ push }), useRoute: () => route }))
vi.mock('../../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  api.get.mockImplementation(path => {
    if (path === '/domains?environment_id=2') return Promise.resolve([{ id: 1, environment_id: 2, hostname: 'api.example.com', namespace: 'production', issuer_ref: 'letsencrypt-prod', tls_secret_name: 'cylism-domain-1-tls', certificate_name: 'cylism-domain-1', certificate: { status: 'Ready', renewal_time: '2026-10-01T00:00:00Z' }, application_count: 0 }])
    if (path === '/projects') return Promise.resolve([{ id: 1, environments: [{ id: 2, name: 'production', namespace: 'production', cluster_id: 7 }] }])
    if (path === '/clusters/7/k8s/certs/issuers') return Promise.resolve([{ name: 'letsencrypt-prod', kind: 'ClusterIssuer', ready: true }])
    if (path === '/domains/dns-options?environment_id=2') return Promise.resolve({ connections: [{ id: 3, name: 'Cloud', zones: [{ name: 'example.com' }] }], nodes: [{ id: 5, name: 'node-a', ip: '203.0.113.5' }] })
    if (path === '/domains/adoptable-cloud-domains?environment_id=2') return Promise.resolve([{ hostname: 'legacy.example.com', cloud_connection_id: 3, dns_zone: 'example.com', addresses: ['203.0.113.9'] }])
    return Promise.resolve([])
  })
})

describe('Domains view', () => {
  it('returns to the originating application tab', async () => {
    route.query = { project_id: '1', environment_id: '2', application_id: '4', return_tab: 'access' }
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(wrapper.find('.back-link').text()).toContain('返回应用')
    await wrapper.find('.back-link').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/applications/4', query: { tab: 'access' } })
    wrapper.unmount()
    route.query = { project_id: '1', environment_id: '2' }
  })

  it('renders managed certificate state', async () => {
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await nextTick()
    expect(wrapper.text()).toContain('api.example.com')
    expect(wrapper.text()).toContain('已就绪')
    expect(wrapper.text()).toContain('cylism-domain-1-tls')
    expect(wrapper.text()).not.toContain('关联历史域名')
  })

  it('opens the selected domain signing process and preserves the application return path', async () => {
    route.query = { project_id: '1', environment_id: '2', application_id: '4', return_tab: 'access' }
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.findAll('.action-cell .btn').find(button => button.text() === '查看签发过程').trigger('click')
    expect(push).toHaveBeenCalledWith({
      path: '/applications/domains/1/operations',
      query: { project_id: '1', environment_id: '2', application_id: '4', return_tab: 'access', hostname: 'api.example.com' },
    })
    wrapper.unmount()
    route.query = { project_id: '1', environment_id: '2' }
  })

  it('loads ready ClusterIssuers before creating a domain in the selected environment', async () => {
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.get('.btn-primary').trigger('click')
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/clusters/7/k8s/certs/issuers', undefined)
    expect(wrapper.text()).toContain('新建域名')
    expect(wrapper.text()).toContain('example.com')
    expect(wrapper.text()).toContain('letsencrypt-prod')
  })

  it('creates DNS and certificate bindings from the selected zone and nodes', async () => {
    api.post.mockResolvedValue({})
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.get('.domain-toolbar .btn-primary').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.get('input[placeholder="api"]').setValue('orders')
    await wrapper.get('.modal form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(api.post).toHaveBeenCalledWith('/domains', expect.objectContaining({ hostname: 'orders.example.com', cloud_connection_id: 3, dns_zone: 'example.com', node_ids: [5] }))
  })

  it('saves an adoption draft without triggering DNS cutover', async () => {
    api.post.mockResolvedValue({})
    const wrapper = mount(Domains)
    await new Promise(resolve => setTimeout(resolve, 20))
    await wrapper.findAll('.domain-toolbar button').find(button => button.text() === '接管已有解析').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 20))
    const modal = wrapper.findAll('.modal').find(item => item.text().includes('接管已有解析'))
    await modal.find('select').setValue('3:example.com:legacy.example.com')
    await modal.find('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(api.post).toHaveBeenCalledWith('/domains/adopt', expect.objectContaining({ hostname: 'legacy.example.com', environment_id: 2, node_ids: [5] }))
    expect(api.post).not.toHaveBeenCalledWith('/domains/1/cutover')
    wrapper.unmount()
  })

  it('shows transient certificate states as issuing and refreshes them', async () => {
    vi.useFakeTimers()
    api.get.mockImplementation(path => {
      if (path === '/domains?environment_id=2') return Promise.resolve([{ id: 1, environment_id: 2, hostname: 'api.example.com', namespace: 'production', certificate_ownership: 'managed', certificate: { status: 'Issuing', reason: 'DoesNotExist' } }])
      if (path === '/projects') return Promise.resolve([{ id: 1, environments: [{ id: 2, name: 'production', namespace: 'production' }] }])
      return Promise.resolve([])
    })
    const wrapper = mount(Domains)
    await vi.runAllTicks()
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.text()).toContain('签发中')
    expect(wrapper.text()).toContain('DoesNotExist')
    const initialRequests = api.get.mock.calls.filter(([path]) => path === '/domains?environment_id=2').length
    await vi.advanceTimersByTimeAsync(5000)
    expect(api.get.mock.calls.filter(([path]) => path === '/domains?environment_id=2')).toHaveLength(initialRequests + 1)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    expect(api.get.mock.calls.filter(([path]) => path === '/domains?environment_id=2')).toHaveLength(initialRequests + 1)
    vi.useRealTimers()
  })
})
