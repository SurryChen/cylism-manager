import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterHub from './cluster/ClusterHub.vue'
import ResourceHub from './resources/ResourceHub.vue'
import NetworkHub from './network/NetworkHub.vue'
import { getClusterAgentStatus } from '../api/clusters.js'

vi.mock('../api/clusters.js', () => ({ getCluster: vi.fn(async id => ({ id: Number(id), name: '生产集群' })), getClusterAgentStatus: vi.fn(async () => ({ state: 'ready' })) }))

const hubSources = [
  'resources/ResourceHub.vue',
  'network/NetworkHub.vue',
  'cluster/ClusterHub.vue',
].map(file => readFileSync(resolve(process.cwd(), 'src/views', file), 'utf8'))

function routerFor(path) {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/cluster', component: ClusterHub },
      { path: '/cluster/:clusterID/resources', component: ResourceHub, props: true },
      { path: '/cluster/:clusterID/network', component: NetworkHub, props: true },
    ],
  })
}

async function mountHub(Component, path, props) {
  const router = routerFor(path)
  await router.push(path)
  await router.isReady()
  return mount(Component, {
    props: props || (Component === NetworkHub ? { clusterID: '2' } : undefined),
    global: {
      plugins: [router],
      stubs: {
        Cluster: { template: '<div>节点内容</div>' },
        AccessCertificates: { template: '<div>访问证书内容</div>' },
        NodeRegistryMirrors: { template: '<div>镜像源内容</div>' },
        ClusterDNS: { template: '<div>DNS 内容</div>' },
        ChartRepositories: { template: '<div>Chart 内容</div>' },
        Workloads: { template: '<div>工作负载内容</div>' },
        Services: { template: '<div>服务内容</div>' },
        Configs: { template: '<div>配置内容</div>' },
        Sites: { template: '<div>路由内容</div>' },
        Certificates: { template: '<div>证书内容</div>' },
      },
    },
  })
}

describe('Infrastructure aggregation hubs', () => {
  beforeEach(() => { document.body.innerHTML = ''; getClusterAgentStatus.mockResolvedValue({ state: 'ready' }) })

  it('shows cluster records and access certificates in the hub', async () => {
    const wrapper = await mountHub(ClusterHub, '/cluster?tab=registry-mirrors')
    expect(wrapper.findAll('[data-testid^="cluster-tab-"]').map(tab => tab.text())).toEqual(['列表', '跨集群服务', '访问证书'])
  })

  it('places the cluster list first and selects it by default', async () => {
    const wrapper = await mountHub(ClusterHub, '/cluster')
    const tabs = wrapper.findAll('[data-testid^="cluster-tab-"]')
    expect(tabs[0].text()).toBe('列表')
    expect(tabs[0].classes()).toContain('is-active')
  })

  it('loads tab contents asynchronously', () => {
    for (const source of hubSources) {
      expect(source).toContain('defineAsyncComponent')
      expect(source).toContain('defineAsyncComponent(() => import(')
    }
  })

  it('switches cluster records and access certificates from the header', async () => {
    const wrapper = await mountHub(ClusterHub, '/cluster?tab=access-certificates')
    expect(wrapper.get('.hub-content').classes()).not.toContain('hub-content--nodes')
    expect(readFileSync(resolve(process.cwd(), 'src/views/cluster/ClusterHub.vue'), 'utf8')).toContain('margin-top: var(--tabbed-page-content-gap)')
    await wrapper.get('[data-testid="cluster-tab-list"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="cluster-tab-list"]').classes()).toContain('is-active')
  })

  it('does not expose cluster-scoped settings from the global hub', async () => {
    const wrapper = await mountHub(ClusterHub, '/cluster?tab=dns')
    expect(wrapper.find('[data-testid="cluster-tab-dns"]').exists()).toBe(false)
  })

  it('shows the selected cluster and a way back to the list', async () => {
    const wrapper = await mountHub(ResourceHub, '/cluster/2/resources', { clusterID: '2' })
    await flushPromises()
    expect(wrapper.get('.section-tabs-header h1').text()).toBe('生产集群')
    expect(wrapper.get('.back-link').attributes('href')).toBe('/cluster?tab=list')
    expect(wrapper.text()).not.toContain('集群详情')
  })

  it('opens workloads by default and can switch to resource configuration', async () => {
    const wrapper = await mountHub(ResourceHub, '/cluster/2/resources')
    expect(wrapper.get('[data-testid="resource-tab-workloads"]').classes()).toContain('is-active')
    await wrapper.get('[data-testid="resource-tab-configs"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="resource-tab-configs"]').classes()).toContain('is-active')
    expect(wrapper.vm.$route.query.tab).toBe('configs')
  })

  it('shows Agent guidance across resource tabs without mounting resource content', async () => {
    getClusterAgentStatus.mockResolvedValue({ state: 'unreachable' })
    const wrapper = await mountHub(ResourceHub, '/cluster/2/resources', { clusterID: '2' })
    await flushPromises()
    expect(wrapper.text()).toContain('Agent 尚未安装或当前无法连接')
    await wrapper.get('[data-testid="resource-tab-configs"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('前往 Agent 管理')
    expect(wrapper.text()).not.toContain('配置内容')
    wrapper.unmount()
  })

  it('opens the requested network certificate tab', async () => {
    const wrapper = await mountHub(NetworkHub, '/cluster/2/network?tab=certificates')
    await flushPromises()
    expect(wrapper.get('[data-testid="network-tab-certificates"]').classes()).toContain('is-active')
    expect(wrapper.text()).toContain('证书内容')
  })
})
