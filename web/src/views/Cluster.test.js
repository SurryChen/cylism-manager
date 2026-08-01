import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Cluster from './Cluster.vue'

const { get, patch } = vi.hoisted(() => ({ get: vi.fn(), patch: vi.fn(() => Promise.resolve({})) }))

get.mockImplementation(path => {
  if (path === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true, roles: 'worker', cpu_cores: 2, memory_mb: 2048 }])
  if (path === '/servers') return Promise.resolve([{ name: '应用节点', k8s_node_name: 'worker-a' }])
  if (path === '/nodes/worker-a/labels') return Promise.resolve({ name: 'worker-a', labels: { 'kubernetes.io/hostname': 'worker-a', team: 'platform' }, protected_keys: ['kubernetes.io/hostname'] })
  return Promise.resolve([])
})

vi.mock('../api/index.js', () => ({ api: { get, post: vi.fn(), patch, delete: vi.fn() } }))

describe('Cluster view', () => {
  it('shows and opens management for custom node labels', async () => {
    const wrapper = mount(Cluster)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('应用节点')
    await wrapper.get('[data-testid="manage-labels-worker-a"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('管理节点标签')
    expect(wrapper.text()).toContain('kubernetes.io/hostname')
    expect(wrapper.find('.label-editor-row input').element.value).toBe('team')
    expect(wrapper.text()).toContain('系统标签')

    await wrapper.get('.modal-actions .btn-primary').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(patch).toHaveBeenCalledWith('/nodes/worker-a/labels', { set: {}, remove: [] })
    expect(wrapper.text()).not.toContain('管理节点标签')
  })
})
