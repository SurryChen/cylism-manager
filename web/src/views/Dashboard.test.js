import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Dashboard from './Dashboard.vue'
import { getDashboardClusters, getDashboardOverview } from '../api/dashboard.js'
import { getMonitoringDashboard, getMonitoringStatus } from '../api/monitoring.js'

vi.mock('../api/dashboard.js', () => ({
  getDashboardClusters: vi.fn(),
  getDashboardOverview: vi.fn(),
}))
vi.mock('../api/monitoring.js', () => ({
  getMonitoringDashboard: vi.fn(),
  getMonitoringStatus: vi.fn(),
}))
vi.mock('./monitoring/MetricTrendChart.vue', () => ({
  default: { template: '<article class="dashboard-trend-chart"><slot name="title" /><slot name="actions" /><slot name="toolbar" /></article>' },
}))

const primary = {
  id: 1, name: '生产集群', health: 'degraded', metrics: {
    nodes_total: 3, nodes_ready: 3, pods_total: 10, pods_ready: 8,
    deployments_total: 4, deployments_ready: 4, services_total: 6,
    namespaces: 3, cpu_cores_total: 12, memory_mb_total: 2048,
  },
}
const secondary = {
  id: 4, name: '预发集群', health: 'healthy', metrics: {
    nodes_total: 2, nodes_ready: 2, pods_total: 5, pods_ready: 5,
    deployments_total: 2, deployments_ready: 2, services_total: 3,
    namespaces: 2, cpu_cores_total: 8, memory_mb_total: 1024,
  },
}
const pending = { id: 2, name: '测试集群', health: 'pending' }
const unavailable = { id: 3, name: '异地集群', health: 'unavailable', error: 'Agent 不可达' }

function render() {
  return mount(Dashboard, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } })
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.resetAllMocks()
  getDashboardClusters.mockResolvedValue([primary, secondary, pending])
  getDashboardOverview.mockResolvedValue({ application_summary: { total_applications: 4 }, recent_logs: [] })
  getMonitoringStatus.mockResolvedValue({ state: 'not_installed' })
  getMonitoringDashboard.mockResolvedValue({ trends: {} })
})

describe('multi-cluster dashboard', () => {
  it('sums resources across deployed clusters without counting pending clusters', async () => {
    const wrapper = render()
    await flushPromises()

    const metrics = wrapper.findAll('.dashboard-resource-metric')
    expect(metrics.map(metric => metric.find('strong').text())).toEqual(['5', '13/15', '6/6', '9', '5', '20 vCPU · 3.0 GiB'])
    expect(wrapper.find('.dashboard-coverage').text()).toBe('已汇总 2/2 个已部署集群')
    expect(wrapper.find('.dashboard-resources').text()).not.toContain('集群概况')
    expect(wrapper.find('.dashboard-table-wrap').exists()).toBe(false)
  })

  it('marks totals as partial when an Agent is unavailable', async () => {
    getDashboardClusters.mockResolvedValue([primary, unavailable])
    const wrapper = render()
    await flushPromises()

    expect(wrapper.find('.dashboard-coverage').text()).toBe('已汇总 1/2 个已部署集群')
    expect(wrapper.find('.dashboard-coverage').classes()).toContain('is-partial')
    expect(wrapper.find('.dashboard-resource-metric strong').text()).toBe('3')
  })

  it('does not present an incomplete metric as a full total', async () => {
    const partial = { ...secondary, metrics: { ...secondary.metrics, services_total: undefined } }
    getDashboardClusters.mockResolvedValue([primary, partial])
    const wrapper = render()
    await flushPromises()

    expect(wrapper.find('.dashboard-coverage').text()).toContain('部分指标不可用')
    expect(wrapper.findAll('.dashboard-resource-metric')[3].find('strong').text()).toBe('—')
    expect(wrapper.findAll('.dashboard-resource-metric')[0].find('strong').text()).toBe('5')
  })

  it('does not query trends when VictoriaMetrics is not installed', async () => {
    const wrapper = render()
    await flushPromises()

    expect(getMonitoringStatus).toHaveBeenCalledWith(expect.objectContaining({ clusterID: '1' }))
    expect(getMonitoringDashboard).not.toHaveBeenCalled()
    expect(wrapper.find('.dashboard-trend-empty').text()).toContain('未安装 VictoriaMetrics')
    expect(wrapper.find('.dashboard-trend-empty a').attributes('href')).toBe('/cluster/1/monitoring')
  })

  it('places the cluster selector first and loads only the selected cluster trends', async () => {
    getMonitoringStatus.mockImplementation(options => Promise.resolve({ state: options.clusterID === '1' ? 'not_installed' : 'ready', ready_replicas: options.clusterID === '1' ? 0 : 1 }))
    const wrapper = render()
    await flushPromises()

    await wrapper.find('.dashboard-trend-empty .dashboard-cluster-select select').setValue('4')
    await flushPromises()

    expect(getMonitoringDashboard).toHaveBeenCalledWith('6h', expect.objectContaining({ clusterID: '4' }))
    const title = wrapper.find('.dashboard-trend-chart .dashboard-trend-title')
    expect(title.element.firstElementChild.classList.contains('dashboard-cluster-select')).toBe(true)
    expect(title.find('h2').text()).toBe('资源趋势')
    expect(wrapper.find('.dashboard-trend-chart').text()).not.toContain('预发集群预发集群')
  })
})
