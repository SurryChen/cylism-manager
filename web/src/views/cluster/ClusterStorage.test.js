import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterStorage from './ClusterStorage.vue'

const { getCluster, getAgentStatus, getInventory, getReferences, getUsage, getClasses, getNamespaces } = vi.hoisted(() => ({
  getCluster: vi.fn(),
  getAgentStatus: vi.fn(),
  getInventory: vi.fn(),
  getReferences: vi.fn(),
  getUsage: vi.fn(),
  getClasses: vi.fn(),
  getNamespaces: vi.fn(),
}))
vi.mock('../../api/clusters.js', () => ({ getCluster, getClusterAgentStatus: getAgentStatus }))
vi.mock('../../api/storage.js', () => ({
  getClusterPersistentVolumeInventory: getInventory,
  getClusterPersistentVolumeReferences: getReferences,
  getClusterPersistentVolumeUsage: getUsage,
  getClusterStorageClasses: getClasses,
  getClusterStorageNamespaces: getNamespaces,
  createClusterPersistentVolumeClaim: vi.fn(),
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
})
