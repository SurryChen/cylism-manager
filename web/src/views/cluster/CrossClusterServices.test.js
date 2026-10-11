import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import CrossClusterServices from './CrossClusterServices.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const { get, post, remove } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), remove: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post, delete: remove } }))

async function mountPage() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/cluster', component: { template: '<div />' } }, { path: '/cluster/:clusterID', component: { template: '<div />' } }] })
  await router.push('/cluster?tab=cross-services')
  await router.isReady()
  return mount(CrossClusterServices, { global: { plugins: [router] }, attachTo: document.body })
}

describe('CrossClusterServices', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    remove.mockReset()
    get.mockImplementation(path => {
      if (path === '/cross-cluster-services') return Promise.resolve([])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: 'A', status: 'ready' }, { id: 2, name: 'B', status: 'ready' }])
      if (path === '/clusters/1/k8s/namespace-names') return Promise.resolve([{ name: 'default' }])
      if (path.includes('/cross-services/precheck')) return Promise.resolve({ ready: true, target_ip: '100.64.0.2' })
      if (path === '/clusters/2/k8s/services?endpoint_count=false') return Promise.resolve([{ namespace: 'default', name: 'orders', cluster_ip: '10.43.0.25', ports: ['TCP:8080'] }])
      return Promise.resolve([])
    })
  })

  it('creates an outgoing Service only after selecting a ready target', async () => {
    post.mockResolvedValue({ id: 1 })
    const wrapper = await mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('暂无跨集群服务')
    await wrapper.get('.cross-toolbar .btn-primary').trigger('click')
    await flushPromises()
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'cross-source-cluster').vm.$emit('update:modelValue', '1')
    await flushPromises()
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'cross-target-cluster').vm.$emit('update:modelValue', '2')
    await flushPromises()
    expect(get).toHaveBeenCalledWith('/clusters/1/cross-services/precheck?target_cluster_id=2', expect.any(Object))
    const menus = wrapper.findAllComponents(SelectMenu)
    menus.find(menu => menu.props('id') === 'cross-target-service').vm.$emit('update:modelValue', 'default/orders')
    menus.find(menu => menu.props('id') === 'cross-source-namespace').vm.$emit('update:modelValue', 'default')
    const nameInput = document.querySelector('#cross-source-name')
    nameInput.value = 'orders-b'
    nameInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.body.textContent).toContain('组网预检通过')
    document.querySelector('#cross-service-form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/1/cross-services', {
      target_cluster_id: 2, target_namespace: 'default', target_name: 'orders', target_service_port: 8080,
      source_namespace: 'default', source_name: 'orders-b', source_port: 8080,
    })
    wrapper.unmount()
  })

  it('lists both directions globally and routes actions through each source cluster', async () => {
    get.mockImplementation(path => {
      if (path === '/cross-cluster-services') return Promise.resolve([
        { id: 5, source_cluster_id: 1, target_cluster_id: 2, source_namespace: 'default', source_name: 'orders-b', source_port: 80, target_namespace: 'default', target_name: 'orders', target_service_port: 8080, status: 'active' },
        { id: 6, source_cluster_id: 2, target_cluster_id: 1, source_namespace: 'default', source_name: 'orders-a', source_port: 80, target_namespace: 'default', target_name: 'orders', target_service_port: 8080, status: 'active' },
      ])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: 'A', status: 'ready' }, { id: 2, name: 'B', status: 'ready' }])
      if (path.endsWith('/check')) return Promise.resolve({ status: 'node_reachable', message: '节点可达' })
      return Promise.resolve([])
    })
    const wrapper = await mountPage()
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('thead th').map(cell => cell.text())).toEqual(['来源集群', '访问名称', '目标集群', '目标 Service', '端口', '状态', '操作'])

    wrapper.findAllComponents(SelectMenu)[0].vm.$emit('update:modelValue', '2')
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('tbody tr').text()).toContain('orders-a')

    await wrapper.get('tbody tr [title="检测 orders-a"]').trigger('click')
    await flushPromises()
    expect(get).toHaveBeenCalledWith('/clusters/2/cross-services/6/check')
    wrapper.unmount()
  })
})
