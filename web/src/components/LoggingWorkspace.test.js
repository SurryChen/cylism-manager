import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LoggingWorkspace from './LoggingWorkspace.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))

beforeEach(() => {
  Object.values(apiMocks).forEach(mock => mock.mockReset())
  apiMocks.get.mockImplementation(path => {
    if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'not_installed', message: '尚未启用容器日志采集' })
    if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: [], pods: [], nodes: [] })
    if (path === '/applications') return Promise.resolve([])
    if (path === '/projects') return Promise.resolve([])
    return Promise.resolve({})
  })
})

describe('LoggingWorkspace', () => {
  it('shows an install form for managed Loki storage', async () => {
    const wrapper = mount(LoggingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], storageClasses: [] } })
    await flushPromises()

    expect(wrapper.text()).toContain('容器日志未启用')
    expect(wrapper.find('input[placeholder="10Gi"]').element.value).toBe('10Gi')
    expect(wrapper.text()).toContain('cylism-loki-data')
  })

  it('loads filters but does not query logs until requested', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'ready', message: '日志采集中', node_name: 'node-a', loki_ready: 1, alloy_ready: 2, alloy_desired: 2, pvc_name: 'cylism-loki-data', storage: '10Gi', retention_days: 14 })
      if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: ['project-demo'], nodes: ['node-a'], pods: [{ name: 'api-123', namespace: 'project-demo', containers: ['api'] }] })
      if (path === '/applications') return Promise.resolve([])
      if (path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })
    apiMocks.post.mockResolvedValue({ lines: [{ timestamp: '2026-08-05T10:00:00Z', line: 'request failed', labels: { namespace: 'project-demo', pod: 'api-123', container: 'api' } }], has_more: false })

    const wrapper = mount(LoggingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], storageClasses: [] } })
    await flushPromises()

    expect(wrapper.text()).toContain('日志检索')
    expect(apiMocks.post).not.toHaveBeenCalledWith('/monitoring/logs/query', expect.anything())
    await wrapper.get('form.logging-query').trigger('submit')
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/monitoring/logs/query', expect.objectContaining({ range: '1h', limit: 200 }))
    expect(wrapper.text()).toContain('request failed')
  })
})
