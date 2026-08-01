import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Cluster from './Cluster.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true, roles: 'worker', version: 'v1.31.0+k3s1', internal_ip: '100.101.1.10', cpu_cores: 4, memory_mb: 8192 }])
      if (url === '/servers') return Promise.resolve([{ id: 1, name: 'srv-a', k8s_node_name: 'worker-a' }])
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

  it('only exposes force drain for failed workers and requires explicit confirmation', async () => {
    vi.clearAllMocks()
    api.get.mockImplementation(url => {
      if (url === '/nodes') return Promise.resolve([
        { name: 'worker-dead', ready: false, health_state: 'failed', roles: 'worker', version: 'v1.31.0+k3s1', internal_ip: '100.101.1.10', cpu_cores: 4, memory_mb: 8192 },
        { name: 'worker-restarting', ready: false, health_state: 'not_ready', roles: 'worker', version: 'v1.31.0+k3s1', internal_ip: '100.101.1.11', cpu_cores: 4, memory_mb: 8192 },
      ])
      if (url === '/servers') return Promise.resolve([])
      if (url === '/nodes/worker-dead/drain-plan') return Promise.resolve({
        evictable: [{ namespace: 'default', name: 'web', owner_kind: 'ReplicaSet' }],
        skipped: [], blocked: [], requires_empty_dir_confirmation: [],
      })
      return Promise.resolve({})
    })
    api.post.mockResolvedValue({
      forced: true,
      deleted: [{ namespace: 'default', name: 'web', reason: '已提交强制删除，控制器将在健康节点重建' }],
      blocked: [], skipped: [], failed: [],
    })
    const wrapper = mount(Cluster, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 50))
    await nextTick()

    expect(wrapper.text()).toContain('故障')
    expect(wrapper.find('[data-testid="force-drain-worker-dead"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="force-drain-worker-restarting"]').exists()).toBe(false)

    await wrapper.get('[data-testid="force-drain-worker-dead"]').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-testid="submit-force-drain"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="force-drain-acknowledge"]').setValue(true)
    await wrapper.get('[data-testid="force-drain-node-name"]').setValue('worker-dead')
    await nextTick()
    expect(wrapper.get('[data-testid="force-drain-acknowledge"]').element.checked).toBe(true)
    expect(wrapper.get('[data-testid="force-drain-node-name"]').element.value).toBe('worker-dead')
    expect(wrapper.get('[data-testid="submit-force-drain"]').element.disabled).toBe(false)
    await wrapper.get('[data-testid="submit-force-drain"]').trigger('click')
    await nextTick()

    expect(api.post).toHaveBeenCalledWith('/nodes/worker-dead/force-drain', {
      acknowledge_risk: true,
      confirm_node_name: 'worker-dead',
      delete_empty_dir_data: false,
    })
    expect(wrapper.text()).toContain('强制驱逐结果')
    expect(wrapper.text()).toContain('default/web')
  })

  it('shows evicted nodes and allows them to rejoin the cluster', async () => {
    vi.clearAllMocks()
    api.get.mockImplementation(url => {
      if (url === '/nodes') return Promise.resolve([{ name: 'worker-drained', ready: true, evicted: true, roles: 'worker', version: 'v1.31.0+k3s1' }])
      if (url === '/servers') return Promise.resolve([])
      return Promise.resolve({})
    })
    api.post.mockResolvedValue({ name: 'worker-drained', evicted: false })
    const wrapper = mount(Cluster, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 50))
    await nextTick()

    expect(wrapper.text()).toContain('已驱逐')
    expect(wrapper.get('[data-testid="rejoin-worker-drained"]').exists()).toBe(true)
    await wrapper.get('[data-testid="rejoin-worker-drained"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/nodes/worker-drained/rejoin')
  })
})
