import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RegistryProxyWorkspace from './RegistryProxyWorkspace.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('RegistryProxyWorkspace', () => {
  it('manages an independent Registry Proxy from the delivery workspace', async () => {
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 2, name: 'worker-a', cluster_role: 'worker', k8s_node_name: 'worker-a' }] : [{
      id: 1,
      name: 'Docker Hub 代理',
      registry: 'docker.io',
      upstream_url: 'https://registry-1.docker.io',
      endpoint_host: '100.64.0.8',
      node_port: 30500,
      node_name: 'worker-a',
      cache_limit_gi: 2,
      cleanup_interval_hours: 24,
      status: 'ready',
    }]))
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('Docker Hub 代理')
    await wrapper.get('[data-testid="create-registry-proxy"]').trigger('click')
    expect(document.body.querySelector('.proxy-modal')).not.toBeNull()
    wrapper.unmount()
  })

  it('migrates a legacy Proxy after confirmation', async () => {
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : [{
      id: 1,
      name: 'Docker Hub 代理',
      registry: 'docker.io',
      resource_name: 'cylism-registry-proxy',
      endpoint_host: '100.64.0.8',
      node_port: 30500,
      node_name: 'worker-a',
      cache_limit_gi: 2,
      cleanup_interval_hours: 24,
      status: 'ready',
    }]))
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="migrate-registry-proxy-1"]').trigger('click')
    await wrapper.get('[data-testid="confirm-registry-proxy-migration"]').trigger('click')

    expect(api.post).toHaveBeenCalledWith('/registry-proxies/1/migrate-resource-name')
    wrapper.unmount()
  })
})
