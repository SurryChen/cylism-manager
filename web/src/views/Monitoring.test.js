import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
  it('uses the shared monitoring workspace header', () => {
    const wrapper = mount(Monitoring)
    expect(wrapper.get('.section-tabs-header').find('h1').text()).toBe('集群监控')
    expect(wrapper.findAll('.section-tab')).toHaveLength(4)
    expect(wrapper.get('.section-tab.is-active').text()).toBe('概览')
  })

  it('offers an installation form with a ready node and managed PVC capacity', async () => {
    const wrapper = mount(Monitoring)
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

    const wrapper = mount(Monitoring)
    await flushPromises()

    expect(wrapper.findAll('.metric-trend-chart')).toHaveLength(4)
    expect(wrapper.text()).toContain('最高 CPU')
    expect(wrapper.text()).toContain('42.5%')
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=6h')
    expect(apiMocks.get).not.toHaveBeenCalledWith('/monitoring/targets')
    expect(wrapper.get('.trend-node-trigger').text()).toContain('全部节点')
    await wrapper.get('.trend-node-trigger').trigger('click')
    expect(wrapper.findAll('.trend-node-option')).toHaveLength(1)
    expect(wrapper.get('.trend-node-option input').element.checked).toBe(true)
    await wrapper.get('.trend-node-option input').setValue(false)
    expect(wrapper.get('.trend-node-option input').element.checked).toBe(false)
    await wrapper.get('.trend-range-select').setValue('24h')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=24h')
    await wrapper.get('.section-tab:nth-child(2)').trigger('click')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith(expect.stringContaining('/monitoring/query?query='))
    expect(apiMocks.get).not.toHaveBeenCalledWith('/monitoring/targets')
    wrapper.unmount()
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
    const wrapper = mount(Monitoring)
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
})
