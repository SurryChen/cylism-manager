import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Cluster from './Cluster.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true, roles: 'worker', version: 'v1.31.0+k3s1', internal_ip: '100.101.1.10', cpu_cores: 4, memory_mb: 8192 }])
      if (url === '/servers') return Promise.resolve([{ id: 1, name: 'srv-a', tailscale_ip: '100.101.1.10', k8s_node_name: 'worker-a' }])
      return Promise.resolve({})
    }),
    post: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
  },
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('Cluster view', () => {
  it('renders node data and mapped server name', async () => {
    const wrapper = mount(Cluster, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 50))
    await nextTick()

    expect(wrapper.text()).toContain('集群节点')
    expect(wrapper.text()).toContain('worker-a')
    expect(wrapper.text()).toContain('srv-a')
    expect(wrapper.text()).toContain('就绪')
  })
})
