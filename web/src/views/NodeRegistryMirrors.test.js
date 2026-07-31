import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import NodeRegistryMirrors from './NodeRegistryMirrors.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

async function settle() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('Node registry mirrors view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue([{
      id: 1,
      name: 'docker-hub-mirror',
      registry: 'docker.io',
      endpoints: '["https://docker.1panel.live"]',
      verification_image: 'docker.io/library/busybox:1.36',
      enabled: true,
      last_verify_status: 'succeeded',
    }])
    api.post.mockResolvedValue({})
  })

  afterEach(() => vi.useRealTimers())

  it('configures a verification image and triggers mirror detection', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    expect(wrapper.text()).toContain('docker.io/library/busybox:1.36')
    expect(wrapper.text()).toContain('可用')

    await wrapper.get('[data-testid="verify-node-registry-mirror-1"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/1/verify')

    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(wrapper.get('input[placeholder="docker.io/library/busybox:1.36"]').exists()).toBe(true)
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
    expect(api.get).toHaveBeenCalledWith('/node-registry-mirrors/1/apply-status')
    expect(wrapper.text()).toContain('worker-a: 成功 - 配置已写入')
    wrapper.unmount()
  })
})
