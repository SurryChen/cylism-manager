import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import Monitoring from './Monitoring.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), delete: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))
vi.mock('../components/MetricTrendChart.vue', () => ({
  default: {
    name: 'MetricTrendChart',
    props: ['series'],
    template: '<div class="metric-trend-chart" />',
  },
}))

function monitoringRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/monitoring', component: Monitoring }],
  })
}

async function mountMonitoring(path = '/monitoring') {
  const router = monitoringRouter()
  await router.push(path)
  await router.isReady()
  return {
    router,
    wrapper: mount(Monitoring, { global: { plugins: [router] } }),
  }
}

beforeEach(() => {
  apiMocks.get.mockReset()
  apiMocks.post.mockReset()
  apiMocks.get.mockImplementation(path => {
    if (path === '/monitoring/status') return Promise.resolve({ state: 'not_installed', message: '尚未安装 VictoriaMetrics' })
    if (path === '/nodes') return Promise.resolve([{ name: 'node-a', ready: true }])
    return Promise.resolve({})
  })
})

describe('Monitoring view', () => {
  it('uses the shared monitoring workspace header', async () => {
    const { wrapper } = await mountMonitoring()
    expect(wrapper.get('.section-tabs-header').find('h1').text()).toBe('集群监控')
    expect(wrapper.findAll('.section-tab')).toHaveLength(5)
    expect(wrapper.get('[data-testid="monitoring-tab-overview"]').classes()).toContain('is-active')
    wrapper.unmount()
  })

  it('offers an installation form with a ready node and managed PVC capacity', async () => {
    const { wrapper } = await mountMonitoring()
    await flushPromises()

    expect(wrapper.text()).toContain('VictoriaMetrics 未安装')
    expect(wrapper.find('.monitoring-install-card').exists()).toBe(true)
    expect(wrapper.text()).toContain('重新检测')
    expect(wrapper.find('select').text()).toContain('node-a')
    expect(wrapper.find('input[placeholder="10Gi"]').element.value).toBe('10Gi')
  })

  it('shows historical node trends in the overview and filters selected nodes', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/status') return Promise.resolve({ state: 'ready', message: '指标采集正常', node_name: 'node-a', data_path: '/data/victoria-metrics', retention_days: 14, node_exporter_ready: 1, node_exporter_desired: 1 })
      if (path === '/nodes') return Promise.resolve([{ name: 'node-a', internal_ip: '10.0.0.1', ready: true }])
      if (path === '/monitoring/targets') return Promise.resolve({ activeTargets: [{ health: 'up' }] })
      if (path.startsWith('/monitoring/dashboard')) return Promise.resolve({ trends: { cpu: { result: [{ metric: { instance: '10.0.0.1:9100' }, values: [[1785000000, '42.5']] }] }, memory: { result: [{ metric: { instance: '10.0.0.1:9100' }, values: [[1785000000, '51.2']] }] }, disk: { result: [{ metric: { instance: '10.0.0.1:9100' }, values: [[1785000000, '32.1']] }] }, network: { result: [{ metric: { instance: '10.0.0.1:9100' }, values: [[1785000000, '1.2']] }] } } })
      return Promise.resolve({ result: [] })
    })

    const { wrapper } = await mountMonitoring()
    await flushPromises()

    expect(wrapper.findAll('.metric-trend-chart')).toHaveLength(4)
    expect(wrapper.text()).toContain('最高 CPU')
    expect(wrapper.text()).toContain('42.5%')
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=6h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(apiMocks.get.mock.calls.map(([path]) => path)).not.toContain('/monitoring/targets')
    expect(apiMocks.get.mock.calls.some(([path]) => path.includes('/monitoring/disk-growth'))).toBe(false)
    expect(wrapper.get('.trend-node-trigger').text()).toContain('全部节点')
    await wrapper.get('.trend-node-trigger').trigger('click')
    expect(wrapper.findAll('.trend-node-option')).toHaveLength(1)
    expect(wrapper.get('.trend-node-option input').element.checked).toBe(true)
    await wrapper.get('.trend-node-option input').setValue(false)
    expect(wrapper.get('.trend-node-option input').element.checked).toBe(false)
    await wrapper.get('.trend-range-select').setValue('24h')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    await wrapper.get('.section-tab:nth-child(2)').trigger('click')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith(expect.stringContaining('/monitoring/query?query='), expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(apiMocks.get.mock.calls.map(([path]) => path)).not.toContain('/monitoring/targets')
    expect(apiMocks.get.mock.calls.some(([path]) => path.includes('/monitoring/disk-growth'))).toBe(false)
    await wrapper.findAll('.section-tab').find(tab => tab.text() === '磁盘').trigger('click')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=6h')
    wrapper.unmount()
  })

  it('keeps metrics available when node exporter coverage is partial', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/status') return Promise.resolve({ state: 'degraded', message: 'VictoriaMetrics 已就绪，但 node-exporter 仅 1/2 个节点就绪', ready_replicas: 1, node_name: 'node-a', retention_days: 14, node_exporter_ready: 1, node_exporter_desired: 2 })
      if (path === '/nodes') return Promise.resolve([{ name: 'node-a', internal_ip: '10.0.0.1', ready: true }, { name: 'node-b', internal_ip: '10.0.0.2', ready: true }])
      if (path.startsWith('/monitoring/dashboard')) return Promise.resolve({ trends: {} })
      return Promise.resolve({ result: [] })
    })

    const { wrapper } = await mountMonitoring()
    await flushPromises()

    expect(wrapper.text()).toContain('node-exporter 仅 1/2 个节点就绪')
    expect(wrapper.findAll('.metric-trend-chart')).toHaveLength(4)
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=6h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('moves monitoring configuration into a settings drawer and updates retention', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/status') return Promise.resolve({ state: 'ready', message: '指标采集正常', node_name: 'node-a', data_path: '/data/victoria-metrics', retention_days: 14, node_exporter_ready: 1, node_exporter_desired: 1 })
      if (path === '/nodes') return Promise.resolve([{ name: 'node-a', internal_ip: '10.0.0.1', ready: true }])
      if (path.startsWith('/monitoring/dashboard')) return Promise.resolve({ trends: {} })
      if (path === '/monitoring/targets') return Promise.resolve({ activeTargets: [] })
      return Promise.resolve({})
    })
    apiMocks.post.mockResolvedValue({ state: 'ready' })
    const { wrapper } = await mountMonitoring()
    await flushPromises()

    await wrapper.get('[title="监控设置"]').trigger('click')
    const drawer = document.body.querySelector('.monitoring-settings-drawer')
    expect(drawer).not.toBeNull()
    const input = drawer.querySelector('input[type="number"]')
    expect(input?.value).toBe('14')
    input.value = 30
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    drawer.querySelector('[data-testid="save-monitoring-config"]').click()
    expect(apiMocks.post).toHaveBeenCalledWith('/monitoring/install', { node_name: 'node-a', retention_days: 30 })
    wrapper.unmount()
  })

  it('opens the requested monitoring tab from the route, follows route changes, and falls back for invalid tabs', async () => {
    const { router, wrapper } = await mountMonitoring('/monitoring?tab=alerts')
    expect(wrapper.get('[data-testid="monitoring-tab-alerts"]').classes()).toContain('is-active')
    await router.push('/monitoring?tab=disk')
    await flushPromises()
    expect(wrapper.get('[data-testid="monitoring-tab-disk"]').classes()).toContain('is-active')
    wrapper.unmount()

    const invalid = await mountMonitoring('/monitoring?tab=unknown')
    expect(invalid.wrapper.get('[data-testid="monitoring-tab-overview"]').classes()).toContain('is-active')
    invalid.wrapper.unmount()
  })

  it('updates the route when a monitoring tab is selected', async () => {
    const { router, wrapper } = await mountMonitoring('/monitoring?range=24h')

    await wrapper.get('[data-testid="monitoring-tab-workloads"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/monitoring')
    expect(router.currentRoute.value.query).toEqual({ range: '24h', tab: 'workloads' })
    expect(wrapper.get('[data-testid="monitoring-tab-workloads"]').classes()).toContain('is-active')
    wrapper.unmount()
  })

  it('keeps the latest trend range when an earlier trend response resolves late', async () => {
    let resolveTwentyFourHours
    const twentyFourHours = new Promise(resolve => { resolveTwentyFourHours = resolve })
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/status') return Promise.resolve({ state: 'ready', node_name: 'node-a' })
      if (path === '/nodes') return Promise.resolve([{ name: 'node-a', internal_ip: '10.0.0.1', ready: true }])
      if (path === '/k8s/storage-classes') return Promise.resolve([])
      if (path === '/monitoring/dashboard?range=6h') {
        return Promise.resolve({ trends: { cpu: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '6']] }] } } })
      }
      if (path === '/monitoring/dashboard?range=24h') return twentyFourHours
      if (path === '/monitoring/dashboard?range=7d') {
        return Promise.resolve({ trends: { cpu: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '7']] }] } } })
      }
      return Promise.resolve({ result: [] })
    })
    const { wrapper } = await mountMonitoring()
    await flushPromises()

    await wrapper.get('.trend-range-select').setValue('24h')
    await wrapper.get('.trend-range-select').setValue('7d')
    await flushPromises()
    resolveTwentyFourHours({ trends: { cpu: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '24']] }] } } })
    await flushPromises()

    expect(wrapper.text()).toContain('7.0%')
    expect(wrapper.text()).not.toContain('24.0%')
    expect(wrapper.text()).not.toContain('6.0%')
    wrapper.unmount()
  })

  it('keeps the loaded monitoring state when a later trend request fails', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/status') return Promise.resolve({ state: 'ready', node_name: 'node-a' })
      if (path === '/nodes') return Promise.resolve([{ name: 'node-a', internal_ip: '10.0.0.1', ready: true }])
      if (path === '/k8s/storage-classes') return Promise.resolve([])
      if (path === '/monitoring/dashboard?range=6h') {
        return Promise.resolve({ trends: { cpu: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '6']] }] } } })
      }
      if (path === '/monitoring/dashboard?range=24h') return Promise.reject(new Error('trend unavailable'))
      return Promise.resolve({ result: [] })
    })
    const { wrapper } = await mountMonitoring()
    await flushPromises()

    await wrapper.get('.trend-range-select').setValue('24h')
    await flushPromises()

    expect(wrapper.text()).toContain('已就绪')
    expect(wrapper.text()).toContain('6.0%')
    expect(wrapper.text()).toContain('trend unavailable')
    wrapper.unmount()
  })
})
