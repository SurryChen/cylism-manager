import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Monitoring from './Monitoring.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), delete: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))

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

  it('offers an installation form with a ready node and hostPath data directory', async () => {
    const wrapper = mount(Monitoring)
    await flushPromises()

    expect(wrapper.text()).toContain('VictoriaMetrics 未安装')
    expect(wrapper.find('.monitoring-install-card').exists()).toBe(true)
    expect(wrapper.text()).toContain('重新检测')
    expect(wrapper.find('select').text()).toContain('node-a')
    expect(wrapper.find('input[placeholder="/data/victoria-metrics"]').element.value).toBe('/data/victoria-metrics')
  })

  it('shows historical node trends and switches to the node view', async () => {
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
    await wrapper.get('.trend-range-select').setValue('24h')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/dashboard?range=24h')
    await wrapper.get('.section-tab:nth-child(2)').trigger('click')
    await flushPromises()
    expect(wrapper.find('.node-table').exists()).toBe(true)
    expect(wrapper.text()).toContain('node-a')
    wrapper.unmount()
  })
})
