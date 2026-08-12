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
    api.get.mockImplementation(path => {
      if (path === '/servers') return Promise.resolve([{
        id: 11,
        name: 'node-a',
        host: '100.64.0.8',
        k8s_node_name: 'node-a',
        cluster_role: 'worker',
      }])
      if (path === '/registry-proxies') return Promise.resolve([{
        id: 2,
        name: 'Kubernetes Registry',
        registry: 'registry.k8s.io',
        upstream_url: 'https://registry.k8s.io',
        endpoint_host: '100.64.0.8',
        node_port: 30501,
        node_name: 'node-a',
        cache_limit_gi: 2,
        cleanup_interval_hours: 24,
        status: 'ready',
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

  it('configures a verification image and triggers mirror detection', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    expect(wrapper.text()).toContain('docker.io/library/busybox:1.36')
    expect(wrapper.text()).toContain('可用')
    expect(wrapper.text()).toContain('registry.k8s.io')
    expect(wrapper.text()).toContain('https://registry.k8s.io')

    await wrapper.get('[data-testid="diagnose-registry-proxy-2"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/registry-proxies/2/diagnose')

    await wrapper.get('[data-testid="verify-node-registry-mirror-1"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/1/verify')

    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(wrapper.get('input[placeholder="docker.io/library/busybox:1.36"]').exists()).toBe(true)
  })

  it('creates an independent Registry Proxy instance', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    await wrapper.get('[data-testid="create-registry-proxy"]').trigger('click')
    const inputs = wrapper.findAll('.proxy-modal input')
    await inputs[0].setValue('Kubernetes Registry')
    await inputs[1].setValue('registry.k8s.io')
    await inputs[3].setValue('100.64.0.8')
    const select = wrapper.get('.proxy-modal select')
    await select.setValue('node-a')
    await wrapper.get('.proxy-modal form').trigger('submit')

    expect(api.post).toHaveBeenCalledWith('/registry-proxies', expect.objectContaining({
      name: 'Kubernetes Registry',
      registry: 'registry.k8s.io',
      node_name: 'node-a',
    }))
  })

  it('migrates a legacy Docker Hub proxy after confirmation', async () => {
    api.get.mockImplementation(path => {
      if (path === '/servers') return Promise.resolve([])
      if (path === '/registry-proxies') return Promise.resolve([{
        id: 1,
        name: 'Docker Hub 代理',
        registry: 'docker.io',
        upstream_url: 'https://registry-1.docker.io',
        resource_name: 'cylism-registry-proxy',
        endpoint_host: '100.64.0.8',
        node_port: 30500,
        node_name: 'node-a',
        cache_limit_gi: 2,
        cleanup_interval_hours: 24,
        status: 'ready',
      }])
      return Promise.resolve([])
    })
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    await wrapper.get('[data-testid="migrate-registry-proxy-1"]').trigger('click')
    expect(wrapper.text()).toContain('代理会短暂中断')
    await wrapper.get('[data-testid="confirm-registry-proxy-migration"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/registry-proxies/1/migrate-resource-name')
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
