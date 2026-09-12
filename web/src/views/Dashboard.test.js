import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import Dashboard from './Dashboard.vue'
import { getAlertOverview, getDashboardOverview, getKubernetesDashboard } from '../api/dashboard.js'

const dashboardSource = readFileSync(resolve(process.cwd(), 'src/views/Dashboard.vue'), 'utf8')

vi.mock('../api/dashboard.js', () => ({
  getDashboardOverview: vi.fn().mockResolvedValue({
    stats: { total_servers: 2, total_sites: 5, expiring_certs: 1 },
    expiring_certs: [], recent_logs: [],
  }),
  getKubernetesDashboard: vi.fn().mockResolvedValue({
    nodes_total: 3, pods_total: 12, pods_ready: 10,
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

beforeEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('Dashboard view with K8s stats', () => {
  it('centers the constrained dashboard workspace', () => {
    expect(dashboardSource).toContain('.dashboard-page { max-width: 1320px; margin: 0 auto; }')
  })

  it('renders page title', () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    expect(wrapper.find('.page-title').text()).toBe('服务健康度')
  })

  it('renders the cluster strip before the Kubernetes request resolves', () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })

    expect(wrapper.find('.cluster-strip').exists()).toBe(true)
    expect(wrapper.find('.cluster-strip').text()).toContain('集群运行概况')
    expect(wrapper.find('.cluster-strip').text()).toContain('—')
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
  })

  it('renders cluster strip with 6 stat columns', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const strip = wrapper.find('.cluster-strip')
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

    const text = wrapper.find('.cluster-strip').text()
    expect(text).toContain('Deployments')
  })

  it('shows service count in cluster strip', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const text = wrapper.find('.cluster-strip').text()
    expect(text).toContain('Services')
  })

  it('renders metric cards', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const metrics = wrapper.findAll('.metric')
    expect(metrics.length).toBeGreaterThanOrEqual(3)
  })

  it('renders quick action entry cards', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.text()).toContain('常用入口')
    expect(wrapper.text()).toContain('部署 / 更新服务')
    expect(wrapper.text()).toContain('查看监控')
    expect(wrapper.text()).toContain('查看日志')
    expect(wrapper.text()).toContain('服务器与集群')
  })

  it('limits dashboard previews and links to full views', () => {
    expect(dashboardSource).toContain('expiringCerts.slice(0, 3)')
    expect(dashboardSource).toContain('recentLogs.slice(0, 3)')
    expect(dashboardSource).toContain('查看全部证书')
    expect(dashboardSource).toContain('查看全部操作')
  })

  it('renders alert attention and overview blocks', async () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.text()).toContain('待关注事项')
    expect(wrapper.text()).toContain('告警概览')
    expect(wrapper.text()).toContain('存在触发中的告警')
    expect(wrapper.text()).toContain('节点 node-a 不可用')
    expect(wrapper.text()).not.toContain('Alerts')
    expect(wrapper.text()).not.toContain('Attention')
    expect(wrapper.text()).not.toContain('Risk queue')
    expect(wrapper.text()).not.toContain('Change log')
  })
})
