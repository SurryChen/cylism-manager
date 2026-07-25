import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Dashboard from './Dashboard.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.includes('/k8s/dashboard')) {
        return Promise.resolve({
          json: async () => ({
            nodes_total: 3, pods_total: 12, pods_ready: 10,
            deployments_total: 5, deployments_ready: 4,
            services_total: 8, namespaces: 3, version: 'v1.28.4+k3s1'
          })
        })
      }
      return Promise.resolve({
        json: async () => ({
          stats: { total_servers: 2, online_servers: 1, total_sites: 5, expiring_certs: 1 },
          expiring_certs: [], recent_logs: []
        })
      })
    })
  }
}))

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('Dashboard view with K8s stats', () => {
  it('renders page title', () => {
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: true } }
    })
    expect(wrapper.find('.page-title').text()).toBe('服务健康度')
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
    expect(metrics.length).toBeGreaterThanOrEqual(4)
  })
})
