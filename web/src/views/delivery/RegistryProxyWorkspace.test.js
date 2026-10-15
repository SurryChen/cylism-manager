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

  it('opens the deployment form from an empty Proxy workspace', async () => {
    api.get.mockResolvedValue([])
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))

    const emptyState = wrapper.get('.proxy-empty')
    expect(emptyState.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(emptyState.classes()).toContain('surface-card')
    expect(emptyState.text()).toContain('尚未部署 Registry Proxy')
    await wrapper.get('[data-testid="create-registry-proxy"]').trigger('click')
    expect(document.body.querySelector('.proxy-modal')?.textContent).toContain('部署 Registry Proxy')
    wrapper.unmount()
  })

  it('imports an existing proxy in observation mode from a selected cluster', async () => {
    api.get.mockImplementation(path => Promise.resolve(path === '/clusters' ? [{ id: 7, name: '国内' }] : []))
    api.post.mockResolvedValue({})
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="import-registry-proxy"]').trigger('click')
    const form = document.body.querySelector('[data-testid="import-registry-proxy-form"]')
    expect(form.textContent).toContain('国内')
    const cluster = form.querySelector('select')
    cluster.value = '7'
    cluster.dispatchEvent(new Event('change', { bubbles: true }))
    const inputs = form.querySelectorAll('input')
    for (const [index, value] of [[0, 'Docker Hub 旧代理'], [1, 'legacy-registry-proxy'], [3, '100.76.53.48']]) {
      inputs[index].value = value
      inputs[index].dispatchEvent(new Event('input', { bubbles: true }))
    }
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.post).toHaveBeenCalledWith('/registry-proxies/import', expect.objectContaining({ cluster_id: 7, resource_name: 'legacy-registry-proxy', endpoint_host: '100.76.53.48' }))
    wrapper.unmount()
  })

  it('shows observation controls without configuration or cleanup actions', async () => {
    api.get.mockImplementation(path => Promise.resolve(path === '/clusters' ? [{ id: 7, name: '国内' }] : [{ id: 3, cluster_id: 7, management_mode: 'observe', name: '旧代理', registry: 'docker.io', resource_name: 'legacy-registry-proxy', endpoint_host: '100.76.53.48', node_port: 30500, node_name: 'node-a', cache_limit_gi: 10, cleanup_interval_hours: 24, status: 'ready' }]))
    const wrapper = mount(RegistryProxyWorkspace)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.text()).toContain('观察中')
    expect(wrapper.find('[data-testid="edit-registry-proxy-3"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="take-over-registry-proxy-3"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-registry-proxy-observation-3"]').exists()).toBe(true)
    expect(wrapper.find('.proxy-row-actions .btn-danger').exists()).toBe(false)
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
