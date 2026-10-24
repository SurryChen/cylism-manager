import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterStorage from './ClusterStorage.vue'

const { getCluster, getAgentStatus, getProjects, getInventory, getReferences, getUsage, getClasses, getNamespaces, adoptClaim } = vi.hoisted(() => ({
  getCluster: vi.fn(),
  getAgentStatus: vi.fn(),
  getProjects: vi.fn(),
  getInventory: vi.fn(),
  getReferences: vi.fn(),
  getUsage: vi.fn(),
  getClasses: vi.fn(),
  getNamespaces: vi.fn(),
  adoptClaim: vi.fn(),
}))
vi.mock('../../api/clusters.js', () => ({ getCluster, getClusterAgentStatus: getAgentStatus }))
vi.mock('../../api/applications.js', () => ({ getProjects }))
vi.mock('../../api/storage.js', () => ({
  getClusterPersistentVolumeInventory: getInventory,
  getClusterPersistentVolumeReferences: getReferences,
  getClusterPersistentVolumeUsage: getUsage,
  getClusterStorageClasses: getClasses,
  getClusterStorageNamespaces: getNamespaces,
  createClusterPersistentVolumeClaim: vi.fn(),
  adoptClusterPersistentVolumeClaim: adoptClaim,
  deleteClusterPersistentVolumeClaim: vi.fn(),
}))

async function mountStorage() {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/cluster', component: { template: '<div />' } },
    { path: '/cluster/:clusterID/storage', component: ClusterStorage, props: true },
  ] })
  await router.push('/cluster/2/storage')
  await router.isReady()
  return mount(ClusterStorage, { props: { clusterID: '2' }, global: { plugins: [router] } })
}

describe('ClusterStorage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getCluster.mockResolvedValue({ id: 2, name: '生产集群' })
    getAgentStatus.mockResolvedValue({ state: 'ready' })
    getNamespaces.mockResolvedValue([{ name: 'default' }])
    getProjects.mockResolvedValue([])
    adoptClaim.mockResolvedValue({})
    getClasses.mockResolvedValue([{ name: 'local-path' }])
    getInventory.mockResolvedValue({ items: [{ namespace: 'default', name: 'data', managed: true, storage: '5Gi', phase: 'Bound' }], total: 1 })
    getReferences.mockResolvedValue([{ namespace: 'default', name: 'data', references: [] }])
    getUsage.mockResolvedValue([{ namespace: 'default', name: 'data', status: 'available', used_bytes: 1024 }])
  })

  it('reads the selected cluster and enables deletion only after references are loaded', async () => {
    const wrapper = await mountStorage()
    await flushPromises()

    expect(wrapper.get('.section-tabs-header h1').text()).toBe('生产集群')
    expect(wrapper.get('.back-link').attributes('href')).toBe('/cluster?tab=list')
    expect(getNamespaces).toHaveBeenCalledWith('2')
    expect(getInventory).toHaveBeenCalledWith('2', { page: 1, size: 20, namespace: '' })
    expect(getReferences).toHaveBeenCalledWith('2', expect.arrayContaining([expect.objectContaining({ name: 'data' })]))
    expect(wrapper.get('tbody tr').text()).toContain('data')
    expect(wrapper.get('tbody .btn-danger').attributes('disabled')).toBeUndefined()
  })

  it('keeps deletion disabled when workload references cannot be read', async () => {
    getReferences.mockRejectedValue(new Error('引用读取失败'))
    const wrapper = await mountStorage()
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('引用读取失败')
    expect(wrapper.get('tbody .btn-danger').attributes('disabled')).toBeDefined()
  })

  it('does not load storage resources when Agent is unavailable', async () => {
    getAgentStatus.mockResolvedValue({ state: 'unreachable' })
    const wrapper = await mountStorage()
    await flushPromises()
    expect(wrapper.text()).toContain('Agent 尚未安装或当前无法连接')
    expect(getNamespaces).not.toHaveBeenCalled()
    expect(getInventory).not.toHaveBeenCalled()
  })

  it('associates a legacy PVC with an environment in the selected cluster and namespace', async () => {
    getInventory.mockResolvedValueOnce({ items: [{ namespace: 'project-self-app', name: 'trendradar-data', managed: true, storage: '5Gi', phase: 'Bound' }], total: 1 })
      .mockResolvedValueOnce({ items: [{ namespace: 'project-self-app', name: 'trendradar-data', managed: true, environment_id: 2, storage: '5Gi', phase: 'Bound' }], total: 1 })
    getProjects.mockResolvedValue([
      { id: 1, name: '应用项目', environments: [
        { id: 2, name: '生产', namespace: 'project-self-app', cluster_id: 2 },
        { id: 3, name: '其他集群', namespace: 'project-self-app', cluster_id: 1 },
        { id: 4, name: '其他命名空间', namespace: 'default', cluster_id: 2 },
      ] },
    ])
    const wrapper = await mountStorage()
    await flushPromises()

    await wrapper.get('tbody .claim-actions .btn:first-child').trigger('click')
    await flushPromises()
    const modal = wrapper.findAll('.base-modal').find(item => item.text().includes('关联历史存储卷'))
    expect(modal.get('.select-menu-native').findAll('option').map(option => option.text())).toEqual([
      '选择匹配的项目环境', '应用项目 · 生产 · project-self-app',
    ])
    await modal.get('.select-menu-native').setValue('2')
    await modal.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(adoptClaim).toHaveBeenCalledWith('2', 'trendradar-data', { namespace: 'project-self-app', environment_id: 2 })
    expect(wrapper.get('tbody .action-cell').text()).toBe('-')
  })
})
