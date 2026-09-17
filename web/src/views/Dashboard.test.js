import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import Dashboard from './Dashboard.vue'
import { getAlertOverview, getDashboardOverview, getKubernetesDashboard } from '../api/dashboard.js'
import { getMonitoringDashboard } from '../api/monitoring.js'

const dashboardSource = readFileSync(resolve(process.cwd(), 'src/views/Dashboard.vue'), 'utf8')

vi.mock('../api/dashboard.js', () => ({
  getDashboardOverview: vi.fn().mockResolvedValue({
    stats: { total_servers: 2, total_sites: 5, expiring_certs: 1 },
    application_summary: {
      total_applications: 4, successful_applications: 2, releasing_applications: 1, failed_applications: 1, unreleased_applications: 0,
      latest_release: { application_name: 'api', version: 'v2', status: 'succeeded', created_at: '2026-09-14T08:00:00Z' },
    },
    expiring_certs: [], recent_logs: [],
  }),
  getKubernetesDashboard: vi.fn().mockResolvedValue({
    nodes_total: 3, nodes_ready: 3, pods_total: 12, pods_ready: 10,
    deployments_total: 5, deployments_ready: 4,
    services_total: 8, namespaces: 3, version: 'v1.28.4+k3s1',
  }),
  getAlertOverview: vi.fn().mockResolvedValue({
    active: [
      {
        fingerprint: 'alert-1',
        status: 'firing',
        labels: { alertname: 'NodeDown', severity: 'critical', node: 'node-a' },
        annotations: { summary: '节点 node-a 不可用' },
      },
    ],
    resolved: [],
    firing: 1,
    silenced: 2,
  }),
}))

vi.mock('../api/monitoring.js', () => ({
  getMonitoringDashboard: vi.fn().mockResolvedValue({
    trends: {
      cpu: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '42']] }] },
      memory: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '51']] }] },
      disk: { result: [{ metric: { node: 'node-a' }, values: [[1785000000, '32']] }] },
    },
  }),
}))

vi.mock('./monitoring/MetricTrendChart.vue', () => ({
  default: { props: ['title'], template: '<article class="dashboard-trend-chart"><h2>{{ title }}</h2><slot name="actions" /><slot name="toolbar" /></article>' },
}))

beforeEach(() => {
  document.body.innerHTML = ''
  window.localStorage.clear()
  vi.clearAllMocks()
})

describe('Dashboard view with K8s stats', () => {
  it('uses the full available dashboard workspace', () => {
    expect(dashboardSource).toContain('.dashboard-page { width: 100%; max-width: none;')
    expect(dashboardSource).toContain('.dashboard-main-grid { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(280px, 1fr) minmax(240px, .85fr);')
    expect(dashboardSource).toContain('.dashboard-insights-grid { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(250px, .8fr);')
    expect(dashboardSource).toContain('height: calc(100dvh - var(--topbar-height) - var(--shell-padding) - 36px);')
    expect(dashboardSource).toContain('grid-template-rows: auto minmax(280px, 290px) minmax(260px, 1fr);')
    expect(dashboardSource).toContain('@media (min-width: 961px) and (min-height: 800px)')
    expect(dashboardSource).toContain('align-items: stretch;')
    expect(dashboardSource).toContain('dashboard-card-scroll-region')
    expect(dashboardSource).toContain('overscroll-behavior: contain;')
  })

  it('keeps the default quick actions fully visible in the desktop first screen', () => {
    expect(dashboardSource).toContain('<nav class="dashboard-action-list" aria-label="快捷操作">')
    expect(dashboardSource).toContain('.dashboard-action-card { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 10px; align-items: center; min-height: 44px;')
    expect(dashboardSource).toContain('.dashboard-side-panel, .dashboard-activity-panel { overflow: hidden; }')
  })

  it('uses the expanded overview cards for readable operational summaries', () => {
    expect(dashboardSource).toContain('.dashboard-main-grid > .surface-card > :deep(.surface-card-header) { min-height: 28px; margin-bottom: 14px; }')
    expect(dashboardSource).toContain('.dashboard-application-panel { display: flex; min-width: 0; flex-direction: column; padding: var(--space-20); }')
    expect(dashboardSource).toContain('.dashboard-overview-section { display: grid; flex: 1; min-height: 0; grid-template-rows: minmax(0, 1fr) auto; gap: 14px; }')
    expect(dashboardSource).toContain('.dashboard-overview-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); grid-template-rows: repeat(2, minmax(0, 1fr)); align-items: center; }')
    expect(dashboardSource).toContain('.dashboard-overview-metric strong { overflow: hidden; color: var(--text-primary); font: 700 20px/1 var(--font-mono);')
    expect(dashboardSource).toContain('.dashboard-overview-metric span { overflow: hidden; color: var(--text-muted); font-size: 11px;')
    expect(dashboardSource).toContain('.application-status-grid { display: grid; min-height: 0; flex: 1; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-template-rows: repeat(2, minmax(0, 1fr)); gap: 8px 16px; margin: 14px 0; padding: 0; border: 0; }')
    expect(dashboardSource).toContain('.application-status-grid strong { color: var(--text-primary); font: 700 20px/1 var(--font-mono); }')
    expect(dashboardSource).toContain('.attention-list { display: grid; grid-template-rows: repeat(4, minmax(30px, auto)); gap: 3px; }')
    expect(dashboardSource).toContain('.attention-title { min-width: 0; overflow: hidden; color: var(--text-primary); font-size: 12px;')
    expect(dashboardSource).toContain('.attention-value { color: var(--text-primary); font: 700 12px/1 var(--font-mono); }')
    expect(dashboardSource).toContain('.dashboard-activity-panel { overflow: hidden; }')
  })

  it('renders page title', () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    expect(wrapper.find('.page-title').text()).toBe('平台健康度')
  })

  it('renders the cluster strip before the Kubernetes request resolves', () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })

    expect(wrapper.find('.dashboard-health-panel').exists()).toBe(true)
    expect(wrapper.find('.dashboard-overview-grid').exists()).toBe(true)
    expect(wrapper.find('.dashboard-overview-grid').text()).toContain('—')
  })

  it('loads overview, cluster, and alert data independently', async () => {
    mount(Dashboard, {
      global: { stubs: { RouterLink: true } },
    })
    await flushPromises()

    for (const request of [getDashboardOverview, getKubernetesDashboard, getAlertOverview]) {
      expect(request).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    }
    expect(getMonitoringDashboard).toHaveBeenCalledTimes(1)
    expect(getMonitoringDashboard).toHaveBeenCalledWith('6h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('highlights overall health, primary deployment action, and resource trends', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('平台健康度')
    expect(wrapper.text()).toContain('部署服务')
    expect(wrapper.text()).toContain('应用情况')
    expect(wrapper.text()).toContain('资源趋势')
    expect(wrapper.findAll('.dashboard-trend-chart')).toHaveLength(1)
  })

  it('renders cluster resource metrics', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const strip = wrapper.find('.dashboard-overview-grid')
    expect(strip.exists()).toBe(true)
    const text = strip.text()
    expect(text).toContain('节点')
    expect(text).toContain('Pods 就绪')
  })

  it('shows deployment stats in cluster strip', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const text = wrapper.find('.dashboard-overview-grid').text()
    expect(text).toContain('Deployments')
  })

  it('shows service and namespace inventory in the overview', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const text = wrapper.find('.dashboard-overview-grid').text()
    expect(text).toContain('服务')
    expect(text).toContain('命名空间')
  })

  it('shows node health capacity alongside cluster resources', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.find('.dashboard-cluster-health').exists()).toBe(true)
    expect(wrapper.find('.dashboard-cluster-health-bar').exists()).toBe(false)
    expect(wrapper.find('.dashboard-overview-section').classes()).not.toContain('dashboard-card-scroll-region')
    expect(wrapper.findAll('.dashboard-node-health-item')).toHaveLength(1)
    expect(wrapper.find('.dashboard-node-health-item').text()).toContain('Ready')
    expect(wrapper.find('.dashboard-node-health-item').text()).toContain('3')
    expect(wrapper.text()).toContain('节点健康')
    expect(wrapper.text()).toContain('控制面')
    expect(wrapper.text()).toContain('工作节点')
  })

  it('keeps application status categories visible in one row', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.findAll('.application-status-grid > div')).toHaveLength(4)
    expect(wrapper.text()).toContain('未发布')
  })

  it('renders metric cards', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const metrics = wrapper.findAll('.dashboard-overview-grid .dashboard-overview-metric')
    expect(metrics.length).toBe(6)
  })

  it('renders quick action entry cards', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.text()).toContain('快捷操作')
    expect(wrapper.text()).toContain('部署 / 更新服务')
    expect(wrapper.text()).toContain('查看监控')
    expect(wrapper.text()).toContain('查看日志')
    expect(wrapper.text()).toContain('查看工作负载')
    expect(wrapper.find('[aria-label="快捷操作"]').classes()).not.toContain('dashboard-card-scroll-region')
    expect(wrapper.findAll('.dashboard-action-card')).toHaveLength(4)
  })

  it('lets users choose which quick actions appear and stores the preference locally', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await wrapper.find('.dashboard-action-settings').trigger('click')

    expect(wrapper.find('.quick-actions-editor').exists()).toBe(true)
    expect(wrapper.find('.base-modal').exists()).toBe(true)
    expect(wrapper.find('.dashboard-side-panel .quick-actions-editor').exists()).toBe(false)
    const auditOption = wrapper.findAll('.quick-action-option').find(option => option.text().includes('查看日志'))
    await auditOption.find('input').setValue(false)

    expect(wrapper.findAll('.dashboard-action-card').some(card => card.text().includes('查看日志'))).toBe(false)
    expect(JSON.parse(window.localStorage.getItem('cylism.dashboard.quick-actions'))).not.toContain('audit')
  })

  it('switches the dashboard trend between resource types and node detail', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    expect(wrapper.findAll('[aria-label="展示方式"] button').find(button => button.text() === '按节点').classes()).toContain('active')
    const memory = wrapper.findAll('[aria-label="资源类型"] button').find(button => button.text() === '内存')
    await memory.trigger('click')
    await wrapper.findAll('[aria-label="展示方式"] button').find(button => button.text() === '按节点').trigger('click')

    expect(memory.classes()).toContain('active')
    expect(wrapper.find('.dashboard-node-select').exists()).toBe(true)
  })

  it('limits dashboard previews and links to full views', () => {
    expect(dashboardSource).toContain('recentLogs.slice(0, 2)')
    expect(dashboardSource).not.toContain('expiringCerts.slice')
    expect(dashboardSource).toContain('查看全部</router-link>')
  })

  it('renders complete runtime checks and cluster details', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.text()).not.toContain('需要关注')
    expect(wrapper.text()).toContain('运行检查')
    expect(wrapper.text()).toContain('Deployment 就绪')
    expect(wrapper.text()).toContain('Pod 就绪')
    expect(wrapper.text()).toContain('告警')
    expect(wrapper.text()).toContain('证书')
    expect(wrapper.text()).toContain('4/5')
    expect(wrapper.text()).toContain('10/12')
    expect(wrapper.text()).toContain('1 风险')
    expect(wrapper.find('.dashboard-application-panel').text()).toContain('应用情况')
    expect(wrapper.find('.dashboard-application-panel').text()).toContain('4')
    expect(wrapper.find('.dashboard-application-panel').text()).toContain('最近发布')
    expect(wrapper.text()).not.toContain('Alerts')
  })
})
