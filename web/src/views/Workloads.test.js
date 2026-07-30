import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Workloads from './Workloads.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => {
  return {
    api: {
      get: vi.fn().mockImplementation((url) => {
        if (url.includes('/deployments')) return Promise.resolve([])
        if (url.includes('/statefulsets')) return Promise.resolve([])
        if (url.includes('/daemonsets')) return Promise.resolve([])
        return Promise.resolve([])
      }),
      patch: vi.fn().mockResolvedValue({ message: 'ok' }),
      post: vi.fn().mockResolvedValue({ message: 'ok' }),
    }
  }
})

function flush() {
  return new Promise(r => setTimeout(r, 200))
}

beforeEach(() => {
  document.body.innerHTML = ''
  api.get.mockReset()
  api.patch.mockReset()
  api.post.mockReset()
  api.get.mockImplementation(() => Promise.resolve([]))
  api.patch.mockResolvedValue({ message: 'ok' })
  api.post.mockResolvedValue({ message: 'ok' })
})

describe('Workloads view', () => {
  it('shows an inline error banner without opening an alert when loading fails', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((url) => {
      if (url.includes('/deployments')) return Promise.reject(new Error('集群连接失败'))
      return Promise.resolve([])
    })

    const wrapper = mount(Workloads)
    await flush()

    expect(wrapper.find('.k8s-banner').text()).toContain('集群连接失败')
    expect(wrapper.find('.page-header .k8s-banner').exists()).toBe(false)
    expect(wrapper.find('.page-header + .k8s-banner').exists()).toBe(true)
    expect(alertSpy).not.toHaveBeenCalled()
  })

  it('renders resource navigation with workload counts', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const tabs = wrapper.findAll('.resource-tab')
    expect(tabs).toHaveLength(4)
    expect(tabs[0].text()).toContain('Pods')
    expect(tabs[1].text()).toContain('Deployments')
    expect(tabs[2].text()).toContain('StatefulSets')
    expect(tabs[3].text()).toContain('DaemonSets')
  })

  it('shows empty state for deployments when no data', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    expect(wrapper.text()).toContain('暂无 Deployment')
  })

  it('switches to StatefulSets tab', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const stsTab = wrapper.findAll('.resource-tab')[2]
    await stsTab.trigger('click')
    await nextTick()
    await flush()
    expect(wrapper.text()).toContain('暂无 StatefulSet')
  })

  it('switches to DaemonSets tab', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const dsTab = wrapper.findAll('.resource-tab')[3]
    await dsTab.trigger('click')
    await nextTick()
    await flush()
    expect(wrapper.text()).toContain('暂无 DaemonSet')
  })

  it('directly shows pod custom server names and falls back to node names', async () => {
    api.get.mockImplementation((url) => {
      if (url === '/k8s/pods') {
        return Promise.resolve([
          { name: 'api-7f8d', namespace: 'default', status: 'Running', node: 'worker-a', ip: '10.42.0.8', restarts: 0, age: '5m' },
          { name: 'system-5d6c', namespace: 'kube-system', status: 'Pending', node: 'unmanaged-node', ip: '', restarts: 0, age: '1m' },
        ])
      }
      if (url === '/servers') return Promise.resolve([{ id: 1, name: '广州生产节点', k8s_node_name: 'worker-a' }])
      return Promise.resolve([])
    })

    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[0].trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('api-7f8d')
    expect(wrapper.text()).toContain('广州生产节点')
    expect(wrapper.text()).toContain('worker-a')
    expect(wrapper.text()).toContain('unmanaged-node')
  })

  it('filters pods by namespace, server, status, restart count, and name', async () => {
    api.get.mockImplementation((url) => {
      if (url === '/k8s/pods') {
        return Promise.resolve([
          { name: 'orders-api-1', namespace: 'production', status: 'Running', node: 'worker-a', ip: '10.42.0.8', restarts: 2, age: '5m' },
          { name: 'orders-worker-1', namespace: 'production', status: 'Pending', node: 'worker-b', ip: '', restarts: 0, age: '1m' },
          { name: 'frontend-1', namespace: 'staging', status: 'Running', node: 'worker-a', ip: '10.42.0.9', restarts: 0, age: '3m' },
        ])
      }
      if (url === '/servers') return Promise.resolve([
        { id: 1, name: '生产服务器 A', k8s_node_name: 'worker-a' },
        { id: 2, name: '生产服务器 B', k8s_node_name: 'worker-b' },
      ])
      return Promise.resolve([])
    })

    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[0].trigger('click')
    await nextTick()

    await wrapper.find('.pod-filter-trigger').trigger('click')

    await wrapper.find('.pod-filter-namespace').setValue('production')
    await wrapper.find('.pod-filter-node').setValue('worker-a')
    await wrapper.find('.pod-filter-status').setValue('Running')
    await wrapper.find('.pod-restarts-filter input').setValue(true)
    await wrapper.find('.pod-search input').setValue('orders')

    expect(wrapper.text()).toContain('orders-api-1')
    expect(wrapper.text()).not.toContain('orders-worker-1')
    expect(wrapper.text()).not.toContain('frontend-1')
    expect(wrapper.text()).toContain('生产服务器 A')
  })

  it('keeps detailed pod filters collapsed until the filter control is opened', async () => {
    const wrapper = mount(Workloads)
    await flush()
    await wrapper.findAll('.resource-tab')[0].trigger('click')
    await nextTick()

    expect(wrapper.find('.pod-filter-panel').exists()).toBe(false)
    await wrapper.find('.pod-filter-trigger').trigger('click')
    expect(wrapper.find('.pod-filter-panel').exists()).toBe(true)
  })

  it('opens a container terminal only for running Pods with containers', async () => {
    api.get.mockImplementation(url => {
      if (url === '/k8s/pods') return Promise.resolve([{ name: 'orders-api-1', namespace: 'production', status: 'Running', node: 'worker-a', ip: '10.42.0.8', restarts: 0, age: '5m', containers: ['app'] }])
      return Promise.resolve([])
    })
    const wrapper = mount(Workloads, { global: { stubs: { PodTerminal: true } } })
    await flush()
    await wrapper.findAll('.resource-tab')[0].trigger('click')
    await nextTick()

    const terminalButton = wrapper.find('.pod-terminal-action')
    expect(terminalButton.attributes('disabled')).toBeUndefined()
    await terminalButton.trigger('click')
    expect(wrapper.find('pod-terminal-stub').exists()).toBe(true)
  })

  it('renders deployments without accessing a loop variable outside its scope', async () => {
    api.get.mockImplementation((url) => {
      if (url.includes('/deployments')) {
        return Promise.resolve([{
          name: 'demo-api', namespace: 'default', ready: 1, replicas: 1,
          images: ['nginx:1.27'], age: '1h'
        }])
      }
      return Promise.resolve([])
    })

    const wrapper = mount(Workloads)
    await flush()

    expect(wrapper.text()).toContain('demo-api')
  })
})
