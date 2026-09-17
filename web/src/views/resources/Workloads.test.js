import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Workloads from './Workloads.vue'
import {
  getWorkloadDaemonSets,
  getWorkloadDeploymentPods,
  getWorkloadDeploymentRevisions,
  getWorkloadDeployments,
  getWorkloadPods,
  getWorkloadServers,
  getWorkloadStatefulSets,
  rollbackWorkload,
  scaleWorkload,
  updateWorkloadImage,
} from '../../api/kubernetes.js'

vi.mock('../../api/kubernetes.js', () => ({
  getWorkloadDaemonSets: vi.fn(),
  getWorkloadDeploymentPods: vi.fn(),
  getWorkloadDeploymentRevisions: vi.fn(),
  getWorkloadDeployments: vi.fn(),
  getWorkloadPods: vi.fn(),
  getWorkloadServers: vi.fn(),
  getWorkloadStatefulSets: vi.fn(),
  rollbackWorkload: vi.fn(),
  scaleWorkload: vi.fn(),
  updateWorkloadImage: vi.fn(),
}))

function flush() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
  getWorkloadDeployments.mockResolvedValue([])
  getWorkloadStatefulSets.mockResolvedValue([])
  getWorkloadDaemonSets.mockResolvedValue([])
  getWorkloadPods.mockResolvedValue([])
  getWorkloadServers.mockResolvedValue([])
  getWorkloadDeploymentPods.mockResolvedValue([])
  getWorkloadDeploymentRevisions.mockResolvedValue([])
  scaleWorkload.mockResolvedValue({ message: 'ok' })
  updateWorkloadImage.mockResolvedValue({ message: 'ok' })
  rollbackWorkload.mockResolvedValue({ message: 'ok' })
})

describe('Workloads view', () => {
  it('loads inventory through named Kubernetes API functions with AbortSignals', async () => {
    const wrapper = mount(Workloads)
    await flush()

    expect(getWorkloadDeployments).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(getWorkloadStatefulSets).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(getWorkloadDaemonSets).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(getWorkloadPods).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.findAll('.resource-tab')).toHaveLength(4)
    expect(wrapper.text()).toContain('暂无 Pod')
    wrapper.unmount()
  })

  it('retains successful inventory regions when one refresh request fails', async () => {
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    const wrapper = mount(Workloads)
    await flush()
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    expect(wrapper.text()).toContain('api')

    getWorkloadDeployments.mockRejectedValueOnce(new Error('Deployment 刷新失败'))
    await wrapper.find('.icon-button[title="刷新工作负载"]').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('api')
    expect(wrapper.text()).toContain('Deployment 刷新失败')
    wrapper.unmount()
  })

  it('filters Pods by namespace, node, status, restart count, and name', async () => {
    getWorkloadPods.mockResolvedValue([
      { name: 'orders-api-1', namespace: 'production', status: 'Running', node: 'worker-a', ip: '10.42.0.8', restarts: 2, age: '5m' },
      { name: 'orders-worker-1', namespace: 'production', status: 'Pending', node: 'worker-b', ip: '', restarts: 0, age: '1m' },
      { name: 'frontend-1', namespace: 'staging', status: 'Running', node: 'worker-a', ip: '10.42.0.9', restarts: 0, age: '3m' },
    ])
    getWorkloadServers.mockResolvedValue([
      { id: 1, name: '生产服务器 A', k8s_node_name: 'worker-a' },
      { id: 2, name: '生产服务器 B', k8s_node_name: 'worker-b' },
    ])
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.find('.pod-filter-trigger').trigger('click')
    await wrapper.find('.pod-filter-namespace .select-menu-native').setValue('production')
    await wrapper.find('.pod-filter-node .select-menu-native').setValue('worker-a')
    await wrapper.find('.pod-filter-status .select-menu-native').setValue('Running')
    await wrapper.find('.pod-restarts-filter input').setValue(true)
    await wrapper.find('.pod-search input').setValue('orders')

    expect(wrapper.text()).toContain('orders-api-1')
    expect(wrapper.text()).not.toContain('orders-worker-1')
    expect(wrapper.text()).not.toContain('frontend-1')
    expect(wrapper.text()).toContain('生产服务器 A')
    wrapper.unmount()
  })

  it('associates late deployment detail responses with their own workload key', async () => {
    let resolveFirst
    const first = new Promise(resolve => { resolveFirst = resolve })
    getWorkloadDeployments.mockResolvedValue([
      { name: 'first', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] },
      { name: 'second', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:2.0'] },
    ])
    getWorkloadDeploymentPods.mockImplementation((_namespace, name) => name === 'first' ? first : Promise.resolve([{ name: 'second-pod', status: 'Running', node: 'worker' }]))
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    const rows = wrapper.findAll('tbody > tr').filter(row => row.classes('clickable'))
    await rows[0].trigger('click')
    await rows[1].trigger('click')
    resolveFirst([{ name: 'first-pod', status: 'Running', node: 'worker' }])
    await flush()
    await nextTick()

    expect(wrapper.text()).toContain('second-pod')
    expect(wrapper.text()).not.toContain('first-pod')
    wrapper.unmount()
  })

  it('keeps the scale dialog open and releases submitting state on failure', async () => {
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    scaleWorkload.mockRejectedValueOnce(new Error('扩缩容被拒绝'))
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    await wrapper.find('.action-cell .btn').trigger('click')
    await wrapper.find('.modal .btn-primary').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('扩缩容被拒绝')
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.find('.modal .btn-primary').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('awaits inventory refresh after a successful scale operation', async () => {
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    await wrapper.find('.action-cell .btn').trigger('click')
    await wrapper.find('.modal .btn-primary').trigger('click')
    await flush()
    await flush()

    expect(scaleWorkload).toHaveBeenCalledWith('deployment', 'default', 'api', 1)
    expect(getWorkloadDeployments).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.modal').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the image dialog open when updating an image fails', async () => {
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    updateWorkloadImage.mockRejectedValueOnce(new Error('镜像地址无效'))
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    await wrapper.find('.action-cell .btn:nth-child(2)').trigger('click')
    await wrapper.find('.modal .btn-primary').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('镜像地址无效')
    expect(wrapper.find('.modal').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows revisions in a rollback dialog and retains it on rollback failure', async () => {
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    getWorkloadDeploymentRevisions.mockResolvedValue([{ revision: 3, image: 'nginx:1.0', age: '1h' }])
    rollbackWorkload.mockRejectedValueOnce(new Error('回滚失败'))
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    await wrapper.find('.action-cell .btn:nth-child(3)').trigger('click')
    await flush()
    await wrapper.find('.modal .btn-sm').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('回滚失败')
    expect(wrapper.find('.modal').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not update disposed state after a pending detail request resolves', async () => {
    let resolveDetails
    let detailSignal
    getWorkloadDeployments.mockResolvedValue([{ name: 'api', namespace: 'default', replicas: 1, ready: 1, images: ['nginx:1.0'] }])
    getWorkloadDeploymentPods.mockImplementation((_namespace, _name, { signal }) => {
      detailSignal = signal
      return new Promise(resolve => { resolveDetails = resolve })
    })
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[1].trigger('click')
    await wrapper.find('tbody > tr.clickable').trigger('click')
    wrapper.unmount()
    resolveDetails([{ name: 'api-pod', status: 'Running', node: 'worker' }])
    await flush()
    expect(detailSignal.aborted).toBe(true)
  })
})
