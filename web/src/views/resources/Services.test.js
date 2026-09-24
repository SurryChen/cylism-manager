import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Services from './Services.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.includes('/endpoints')) return Promise.resolve([{ name: 'web-slice', address_type: 'IPv4', endpoints: [{ ip: '10.42.0.4', node: 'node-1', pod_name: 'web-abc', port: 8080, ready: true }] }])
      return Promise.resolve([
        { name: 'web-svc', namespace: 'default', type: 'ClusterIP', cluster_ip: '10.43.1.1', ports: ['TCP:80'], endpoint_count: 2, selector: { app: 'web' }, age: '5d' },
        { name: 'metrics', namespace: 'monitoring', type: 'ClusterIP', cluster_ip: '10.43.2.2', ports: ['TCP:9090'], endpoint_count: 0, age: '3d' },
      ])
    })
  }
}))

beforeEach(() => { vi.clearAllMocks(); document.body.innerHTML = '' })

async function mountLoaded() {
  const wrapper = mount(Services)
  await Promise.resolve()
  await nextTick()
  return wrapper
}

describe('Services view', () => {
  it('uses a card toolbar instead of a repeated page title', async () => {
    const wrapper = await mountLoaded()
    expect(wrapper.find('.page-title').exists()).toBe(false)
    expect(wrapper.find('.service-namespace-filter').exists()).toBe(true)
    expect(wrapper.find('[aria-label="刷新服务"]').exists()).toBe(true)
  })

  it('renders a lightweight service list without endpoint status', async () => {
    const wrapper = await mountLoaded()
    expect(api.get).toHaveBeenCalledWith('/k8s/services?endpoint_count=false', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('Cluster IP')
    expect(wrapper.findAll('th').map(cell => cell.text())).not.toContain('端点')
    expect(wrapper.text()).toContain('查看端点')
  })

  it('filters services by namespace', async () => {
    const wrapper = await mountLoaded()
    await wrapper.find('.service-namespace-filter .select-menu-trigger').trigger('click')
    await wrapper.findAll('.select-menu-option').find(option => option.text() === 'monitoring').trigger('click')
    expect(wrapper.text()).toContain('metrics')
    expect(wrapper.text()).not.toContain('web-svc')
  })

  it('opens endpoint details in a modal', async () => {
    const wrapper = await mountLoaded()
    await wrapper.find('[data-testid="view-service-endpoints-default/web-svc"]').trigger('click')
    await Promise.resolve()
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/k8s/services/default/web-svc/endpoints', undefined)
    expect(wrapper.text()).toContain('服务端点 · default/web-svc')
    expect(wrapper.text()).toContain('web-slice')
    expect(wrapper.text()).toContain('10.42.0.4')
  })
})
