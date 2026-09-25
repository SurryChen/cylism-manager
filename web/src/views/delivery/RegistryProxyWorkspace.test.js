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
    const catalog = wrapper.get('.proxy-list-card')
    expect(catalog.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(catalog.get('.proxy-list-toolbar').text()).toContain('1 个代理')
    expect(wrapper.find('.proxy-instance').exists()).toBe(false)
    expect(wrapper.find('.workspace-heading').exists()).toBe(false)
    expect(catalog.get('thead').text()).toContain('代理名称')
    expect(catalog.get('thead').text()).toContain('Registry')
    expect(catalog.get('thead').text()).toContain('上游')
    expect(catalog.get('thead').text()).toContain('节点入口')
    expect(catalog.get('thead').text()).toContain('连通性')
    expect(catalog.get('.proxy-connectivity').text()).toContain('出网正常')
    expect(catalog.get('.proxy-connectivity').attributes('title')).toContain('代理 Pod 可访问上游 Registry')
    const upstream = catalog.findAll('.overflow-tooltip-trigger')[2]
    Object.defineProperties(upstream.element, { clientWidth: { configurable: true, value: 80 }, scrollWidth: { configurable: true, value: 240 } })
    await upstream.trigger('mouseenter', { clientX: 80, clientY: 120 })
    expect(document.body.querySelector('.overflow-tooltip-content')?.textContent).toBe('https://registry-1.docker.io')
    await upstream.trigger('mouseleave')
    expect(document.body.querySelector('.overflow-tooltip-content')).toBeNull()
    expect(wrapper.find('.proxy-error').exists()).toBe(false)
    expect(api.get).not.toHaveBeenCalledWith('/servers', expect.anything())
    await wrapper.get('[data-testid="create-registry-proxy"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(document.body.querySelector('.proxy-modal')).not.toBeNull()
    expect(document.body.querySelector('.proxy-modal').textContent).toContain('DNS 服务器')
    expect(api.get).toHaveBeenCalledWith('/servers', expect.objectContaining({ signal: expect.any(AbortSignal) }))
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

  it('saves DNS from the proxy configuration modal', async () => {
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
      dns_servers: ['8.8.8.8'],
      status: 'ready',
    }]))
    api.put.mockResolvedValue({})
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[data-testid="edit-registry-proxy-1"]').trigger('click')
    const dnsInput = document.body.querySelector('input[placeholder^="留空使用集群 DNS"]')
    dnsInput.value = '1.1.1.1, 8.8.4.4'
    dnsInput.dispatchEvent(new Event('input', { bubbles: true }))
    document.body.querySelector('.proxy-modal form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.put).toHaveBeenCalledWith('/registry-proxies/1', expect.objectContaining({ dns_servers: ['1.1.1.1', '8.8.4.4'] }))
    wrapper.unmount()
  })
})
