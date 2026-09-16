import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import NodeRegistryMirrors from './NodeRegistryMirrors.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

async function settle() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('Node registry mirrors view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockImplementation(path => {
      if (path === '/servers') return Promise.resolve([{
        id: 11,
        name: 'node-a',
        host: '100.64.0.8',
        k8s_node_name: 'node-a',
        cluster_role: 'worker',
      }])
      return Promise.resolve([{
        id: 1,
        name: 'docker-hub-mirror',
        registry: 'docker.io',
        endpoints: '["https://docker.1panel.live"]',
        verification_image: 'docker.io/library/busybox:1.36',
        enabled: true,
        last_verify_status: 'succeeded',
      }])
    })
    api.post.mockResolvedValue({})
  })

  afterEach(() => vi.useRealTimers())

  it('configures a verification image and triggers mirror detection without loading Proxy workloads', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    expect(wrapper.text()).toContain('docker.io/library/busybox:1.36')
    expect(wrapper.text()).toContain('可用')
    expect(wrapper.text()).not.toContain('自建 Registry Proxy')
    expect(api.get).not.toHaveBeenCalledWith('/registry-proxies', expect.anything())

    await wrapper.get('[data-testid="verify-node-registry-mirror-1"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/1/verify')

    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(document.body.querySelector('input[placeholder="docker.io/library/busybox:1.36"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('renders the mirror editor in the document body above the app navigation', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()
    await wrapper.get('[data-testid="verify-node-registry-mirror-1"]').trigger('click')
    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(document.body.querySelector('.mirror-overlay')).not.toBeNull()
    expect(document.body.querySelector('.mirror-overlay').parentElement).toBe(document.body)
    wrapper.unmount()
  })

  it('shows a confirmation dialog after saving a mirror', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()
    await wrapper.get('[data-testid="edit-node-registry-mirror-1"]').trigger('click')
    document.body.querySelector('.mirror-modal .modal-actions .btn-primary').click()
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.put).toHaveBeenCalledWith('/node-registry-mirrors/1', expect.objectContaining({
      registry: 'docker.io',
      verification_image: 'docker.io/library/busybox:1.36',
    }))
    expect(document.body.querySelector('.mirror-notice-modal')).not.toBeNull()
    expect(document.body.querySelector('.mirror-notice-modal').textContent).toContain('节点镜像源已保存')
    wrapper.unmount()
  })

  it('submits only selected cluster nodes and refreshes their apply status', async () => {
    vi.useFakeTimers()
    api.get.mockImplementation(path => {
      if (path === '/servers') return Promise.resolve([
        { id: 11, name: 'worker-a', host: '10.0.0.11', k8s_node_name: 'worker-a', cluster_role: 'worker' },
        { id: 12, name: 'unmanaged', host: '10.0.0.12', cluster_role: '' },
      ])
      if (path === '/node-registry-mirrors/1/apply-status') return Promise.resolve({
        id: 1,
        last_apply_status: 'succeeded',
        node_statuses: [{ server_id: 11, status: 'success', detail: '配置已写入', server: { name: 'worker-a' } }],
      })
      return Promise.resolve([{
        id: 1,
        name: 'docker-hub-mirror',
        registry: 'docker.io',
        endpoints: '["https://docker.1panel.live"]',
        enabled: true,
      }])
    })
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    await wrapper.get('[data-testid="apply-node-registry-mirror-1"]').trigger('click')
    expect(wrapper.text()).toContain('worker-a')
    expect(wrapper.text()).not.toContain('unmanaged')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-testid="submit-node-registry-apply"]').trigger('click')
    await settle()
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/1/apply', { server_ids: [11] })

    await vi.advanceTimersByTimeAsync(2000)
    const applyStatusCall = api.get.mock.calls.find(([path]) => path === '/node-registry-mirrors/1/apply-status')
    expect(applyStatusCall[1]).toEqual(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('worker-a: 成功 - 配置已写入')
    const statusRequests = api.get.mock.calls.filter(([path]) => path === '/node-registry-mirrors/1/apply-status').length
    await vi.advanceTimersByTimeAsync(2000)
    expect(api.get.mock.calls.filter(([path]) => path === '/node-registry-mirrors/1/apply-status')).toHaveLength(statusRequests)
    wrapper.unmount()
  })

  it('aborts an in-flight apply status read when the page unmounts', async () => {
    vi.useFakeTimers()
    let statusSignal
    api.get.mockImplementation((path, options) => {
      if (path === '/servers') return Promise.resolve([])
      if (path === '/node-registry-mirrors') return Promise.resolve([{ id: 1, last_apply_status: 'applying', endpoints: '[]' }])
      if (path === '/node-registry-mirrors/1/apply-status') {
        statusSignal = options?.signal
        return new Promise(() => {})
      }
      return Promise.resolve([])
    })
    const wrapper = mount(NodeRegistryMirrors)
    await settle()
    await vi.advanceTimersByTimeAsync(2000)
    wrapper.unmount()
    expect(statusSignal?.aborted).toBe(true)
  })

  it('explains that application writes the complete enabled configuration', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()
    await wrapper.get('[data-testid="apply-node-registry-mirror-1"]').trigger('click')

    expect(wrapper.text()).toContain('全部启用的镜像源规则')
    expect(wrapper.text()).toContain('完整的 registries.yaml')
    wrapper.unmount()
  })
})
