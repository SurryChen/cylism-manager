import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ApplicationRuntime from './ApplicationRuntime.vue'

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/applications/4/runtime') return Promise.resolve({ status: 'running', ready_replicas: 1, desired_replicas: 1 })
      if (path === '/applications/4/pods') return Promise.resolve({ metrics_available: true, pods: [{ name: 'api-1', namespace: 'shop-prod', phase: 'Running', ready: true, node: 'worker-a', restarts: 2, containers: ['api'], cpu_millicores: 75, memory_bytes: 134217728 }] })
      if (path.startsWith('/applications/4/pods/api-1/logs?')) return Promise.resolve({ logs: '2026-10-10T00:00:00Z ready' })
      return Promise.resolve([])
    }),
  },
}))

afterEach(() => vi.clearAllMocks())

describe('ApplicationRuntime', () => {
  it('shows the running Pod metrics and opens its logs', async () => {
    const wrapper = mount(ApplicationRuntime, {
      props: { applicationId: 4, clusterId: 2, releases: [{ sequence: 3, status: 'failed', version: '2.0' }, { sequence: 2, status: 'succeeded', version: '1.0' }] },
      global: { stubs: { PodTerminal: true } },
    })
    expect(wrapper.text()).toContain('正在读取运行状态')
    expect(wrapper.find('.runtime-summary').exists()).toBe(false)
    await flushPromises()

    expect(wrapper.text()).toContain('运行版本 1.0')
    expect(wrapper.text()).toContain('api-1')
    expect(wrapper.text()).toContain('75 m')
    expect(wrapper.text()).toContain('128 MiB')
    await wrapper.get('[aria-label="查看 api-1 的日志"]').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('.pod-log-modal')?.textContent).toContain('ready')
    wrapper.unmount()
  })
})
