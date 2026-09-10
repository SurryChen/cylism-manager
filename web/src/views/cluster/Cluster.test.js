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

vi.mock('../../api/index.js', () => ({ api: { get, post: vi.fn(), patch, delete: vi.fn() } }))

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

  it('shows a local error when cluster inventory cannot be loaded', async () => {
    get.mockRejectedValueOnce(new Error('集群连接失败'))
    const wrapper = mount(Cluster)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('集群连接失败')
  })

  it('keeps the labels modal open and scopes a save failure to it', async () => {
    get.mockImplementation(path => {
      if (path === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true, roles: 'worker' }])
      if (path === '/servers') return Promise.resolve([])
      if (path === '/nodes/worker-a/labels') return Promise.resolve({ labels: { team: 'platform' }, protected_keys: [] })
      return Promise.resolve([])
    })
    patch.mockRejectedValueOnce(new Error('标签保存被拒绝'))
    const wrapper = mount(Cluster)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="manage-labels-worker-a"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('.modal-actions .btn-primary').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('标签保存被拒绝')
    expect(wrapper.text()).toContain('管理节点标签')
    wrapper.unmount()
  })

  it('keeps only the latest node label response when selection changes quickly', async () => {
    let resolveFirstLabels
    const firstLabels = new Promise(resolve => { resolveFirstLabels = resolve })
    get.mockImplementation(path => {
      if (path === '/nodes') return Promise.resolve([
        { name: 'worker-a', ready: true, roles: 'worker' },
        { name: 'worker-b', ready: true, roles: 'worker' },
      ])
      if (path === '/servers') return Promise.resolve([])
      if (path === '/nodes/worker-a/labels') return firstLabels
      if (path === '/nodes/worker-b/labels') return Promise.resolve({ labels: { team: 'latest' }, protected_keys: [] })
      return Promise.resolve([])
    })
    const wrapper = mount(Cluster)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="manage-labels-worker-a"]').trigger('click')
    await wrapper.get('[data-testid="manage-labels-worker-b"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    resolveFirstLabels({ labels: { team: 'stale' }, protected_keys: [] })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('worker-b')
    expect(wrapper.find('.label-editor-row input').element.value).toBe('team')
    expect(wrapper.findAll('.label-editor-row input')[1].element.value).toBe('latest')
    wrapper.unmount()
  })
})
