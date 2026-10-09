import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterList from './ClusterList.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const { get, post, put, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), remove: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post, put, delete: remove } }))

async function mountList() {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/cluster', component: { template: '<div />' } },
    { path: '/cluster/:clusterID', component: { template: '<div />' } },
    { path: '/cluster/:clusterID/resources', component: { template: '<div />' } },
    { path: '/cluster/:clusterID/network', component: { template: '<div />' } },
    { path: '/cluster/:clusterID/storage', component: { template: '<div />' } },
    { path: '/cluster/:clusterID/monitoring', component: { template: '<div />' } },
  ] })
  await router.push('/cluster')
  await router.isReady()
  return { wrapper: mount(ClusterList, { global: { plugins: [router] } }), router }
}

describe('ClusterList', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); put.mockReset(); remove.mockReset() })

  it('renders one table row per stored cluster', async () => {
    get.mockResolvedValueOnce([
      { id: 1, name: '生产集群', distribution: 'k3s', status: 'ready', created_at: '2026-10-02T08:00:00Z' },
      { id: 2, name: '测试集群', distribution: 'kubernetes', status: 'draft', created_at: '2026-10-01T08:00:00Z' },
    ])
    const { wrapper } = await mountList()
    await flushPromises()

    expect(get).toHaveBeenCalledWith('/clusters', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.text()).toContain('生产集群')
    expect(wrapper.text()).toContain('K3s')
    expect(wrapper.text()).toContain('K8s')
    expect(wrapper.text()).toContain('共 2 个集群')
    expect(wrapper.get('.cluster-name-link').attributes('href')).toBe('/cluster/1')
    expect(wrapper.get('[aria-label="查看 生产集群 的 Kubernetes 资源"]').attributes('href')).toBe('/cluster/1/resources')
    expect(wrapper.get('[aria-label="查看 生产集群 的网络访问"]').attributes('href')).toBe('/cluster/1/network')
    expect(wrapper.get('[aria-label="查看 生产集群 的存储卷"]').attributes('href')).toBe('/cluster/1/storage')
    expect(wrapper.get('[aria-label="查看 生产集群 的监控"]').attributes('href')).toBe('/cluster/1/monitoring')
    expect(wrapper.findAll('thead th').slice(-2).map(cell => cell.text())).toEqual(['集群操作', '信息维护'])
    expect(wrapper.findAll('tbody tr:first-child .cluster-operation-column .icon-button')).toHaveLength(5)
    expect(wrapper.findAll('tbody tr:first-child .cluster-maintenance-column .icon-button')).toHaveLength(2)
    expect(wrapper.get('[aria-label="集群运维：生产集群"]').attributes('href')).toBe('/cluster/1')
    const hints = [
      '查看 生产集群 的 Kubernetes 资源',
      '查看 生产集群 的网络访问',
      '查看 生产集群 的存储卷',
      '查看 生产集群 的监控',
      '集群运维：生产集群',
      '修改 生产集群 的集群信息',
      '删除集群 生产集群',
    ]
    const actions = wrapper.findAll('tbody tr:first-child .icon-action-tooltip')
    for (const [index, action] of actions.entries()) {
      await action.trigger('mouseenter', { clientX: 100, clientY: 100 })
      expect(document.body.querySelector('[role="tooltip"]')?.textContent).toBe(hints[index])
      await action.trigger('mouseleave')
      expect(document.body.querySelector('[role="tooltip"]')).toBeNull()
    }
  })

  it('shows an empty state when no clusters are registered', async () => {
    get.mockResolvedValueOnce([])
    const { wrapper } = await mountList()
    await flushPromises()
    expect(wrapper.text()).toContain('暂无集群')
  })

  it('shows a recoverable error when loading fails', async () => {
    get.mockRejectedValueOnce(new Error('无法读取集群'))
    const { wrapper } = await mountList()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('无法读取集群')
    expect(wrapper.text()).not.toContain('暂无集群')
    get.mockResolvedValueOnce([])
    await wrapper.get('[title="刷新集群列表"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('creates a logical cluster and opens its detail page', async () => {
    get.mockResolvedValueOnce([])
    post.mockResolvedValueOnce({ id: 9, name: '生产集群', distribution: 'k3s', status: 'draft' })
    const { wrapper, router } = await mountList()
    await flushPromises()

    await wrapper.get('.cluster-list-actions .btn-primary').trigger('click')
    await wrapper.get('#cluster-name').setValue(' 生产集群 ')
    expect(wrapper.get('[role="dialog"] .btn-primary').attributes('disabled')).toBeDefined()
    const typeMenu = wrapper.findComponent(SelectMenu)
    expect(typeMenu.props('options')).toEqual([
      { value: 'k3s', label: 'K3s', description: '当前支持' },
      { value: 'kubernetes', label: 'K8s', description: '暂未支持', disabled: true },
    ])
    typeMenu.vm.$emit('update:modelValue', 'k3s')
    await wrapper.get('#create-cluster-form').trigger('submit')
    await flushPromises()

    expect(post).toHaveBeenCalledWith('/clusters', { name: '生产集群', distribution: 'k3s' })
    expect(router.currentRoute.value.path).toBe('/cluster/9')
  })

  it('keeps the creation form open after a duplicate name', async () => {
    get.mockResolvedValueOnce([])
    post.mockRejectedValueOnce(new Error('集群名称已存在'))
    const { wrapper } = await mountList()
    await flushPromises()

    await wrapper.get('.cluster-list-actions .btn-primary').trigger('click')
    await wrapper.get('#cluster-name').setValue('生产集群')
    wrapper.findComponent(SelectMenu).vm.$emit('update:modelValue', 'k3s')
    await wrapper.get('#create-cluster-form').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="dialog"]').text()).toContain('集群名称已存在')
  })

  it('edits a cluster name and blocks deletion errors in the confirmation dialog', async () => {
    get.mockResolvedValueOnce([{ id: 1, name: '旧名称', distribution: 'k3s', status: 'ready' }])
    put.mockResolvedValueOnce({ id: 1, name: '新名称', distribution: 'k3s', status: 'ready' })
    remove.mockRejectedValueOnce(new Error('请先卸载集群中的全部节点'))
    const { wrapper } = await mountList()
    await flushPromises()
    await wrapper.get('[aria-label="编辑 旧名称"]').trigger('click')
    await wrapper.get('#edit-cluster-name').setValue('新名称')
    await wrapper.get('#edit-cluster-form').trigger('submit')
    await flushPromises()
    expect(put).toHaveBeenCalledWith('/clusters/1', { name: '新名称' })
    expect(wrapper.get('tbody').text()).toContain('新名称')
    await wrapper.get('[aria-label="删除 新名称"]').trigger('click')
    await wrapper.get('[role="dialog"] .btn-danger').trigger('click')
    await flushPromises()
    expect(remove).toHaveBeenCalledWith('/clusters/1')
    expect(wrapper.get('[role="dialog"]').text()).toContain('请先卸载集群中的全部节点')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
  })
})
