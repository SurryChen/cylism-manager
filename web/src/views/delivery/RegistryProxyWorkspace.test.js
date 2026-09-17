import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import RegistryProxyWorkspace from './RegistryProxyWorkspace.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('RegistryProxyWorkspace', () => {
  it('is colocated with the delivery view domain rather than shared components', () => {
    expect(existsSync(resolve(process.cwd(), 'src/views/delivery/RegistryProxyWorkspace.vue'))).toBe(true)
    expect(existsSync(resolve(process.cwd(), 'src/components/RegistryProxyWorkspace.vue'))).toBe(false)
  })

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
      last_diagnostic_status: 'healthy',
      last_diagnostic_error: '代理 Pod 可访问上游 Registry',
    }]))
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('Docker Hub 代理')
    expect(wrapper.get('.proxy-instance').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(wrapper.get('.proxy-instance').classes()).toContain('metric')
    expect(wrapper.get('.proxy-properties').exists()).toBe(true)
    expect(wrapper.find('.workspace-heading h2').exists()).toBe(false)
    const connectivity = wrapper.get('.proxy-connectivity')
    expect(connectivity.text()).toContain('上游连通性')
    expect(connectivity.text()).toContain('出网正常')
    expect(connectivity.text()).toContain('代理 Pod 可访问上游 Registry')
    expect(connectivity.classes()).toContain('is-healthy')
    expect(wrapper.find('.proxy-error').exists()).toBe(false)
    await wrapper.get('[data-testid="create-registry-proxy"]').trigger('click')
    expect(document.body.querySelector('.proxy-modal')).not.toBeNull()
    wrapper.unmount()
  })

  it('uses the shared surface card for an empty Proxy workspace', async () => {
    api.get.mockResolvedValue([])
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))

    const emptyState = wrapper.get('.proxy-empty')
    expect(emptyState.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(emptyState.classes()).toContain('surface-card')
    expect(emptyState.text()).toContain('尚未部署 Registry Proxy')
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
