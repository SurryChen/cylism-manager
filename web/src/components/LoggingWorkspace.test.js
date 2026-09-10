import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LoggingWorkspace from './LoggingWorkspace.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }))
function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => { resolve = resolvePromise; reject = rejectPromise })
  return { promise, resolve, reject }
}

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
    await wrapper.get('[data-testid="log-result-limit"]').setValue('500')
    await wrapper.get('form.logging-query').trigger('submit')
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/monitoring/logs/query', expect.objectContaining({ range: '1h', limit: 500 }), expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('request failed')
  })

  it('keeps log search available when only part of Alloy is ready', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'degraded', message: 'Loki 已就绪，但 Alloy 仅 1/2 个节点就绪；部分节点日志不可用', node_name: 'node-a', loki_ready: 1, alloy_ready: 1, alloy_desired: 2 })
      if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: [], nodes: ['node-a'], pods: [] })
      if (path === '/applications' || path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })

    const wrapper = mount(LoggingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], storageClasses: [] } })
    await flushPromises()

    expect(wrapper.text()).toContain('部分节点日志不可用')
    expect(wrapper.text()).toContain('日志检索')
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/logs/filters', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('sends an exact local time range as UTC values', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'ready', message: '日志采集中', node_name: 'node-a', loki_ready: 1, alloy_ready: 1, alloy_desired: 1, retention_days: 14 })
      if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: [], nodes: [], pods: [] })
      if (path === '/applications' || path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })
    apiMocks.post.mockResolvedValue({ lines: [], has_more: false })

    const wrapper = mount(LoggingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], storageClasses: [] } })
    await flushPromises()

    await wrapper.get('[data-testid="log-time-range"]').setValue('custom')
    await wrapper.get('[data-testid="log-start-time"]').setValue('2026-08-05T09:54:40')
    await wrapper.get('[data-testid="log-end-time"]').setValue('2026-08-05T09:55:00')
    await wrapper.get('form.logging-query').trigger('submit')
    await flushPromises()

    expect(apiMocks.post).toHaveBeenCalledWith('/monitoring/logs/query', expect.objectContaining({
      range: 'custom',
      start_time: new Date('2026-08-05T09:54:40').toISOString(),
      end_time: new Date('2026-08-05T09:55:00').toISOString(),
    }), expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('allows retention settings to be saved while logging is starting', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'installing', message: 'Loki 正在启动', node_name: 'node-a', loki_ready: 0, alloy_ready: 2, alloy_desired: 4, pvc_name: 'cylism-loki-data', storage: '5Gi', retention_days: 14 })
      return Promise.resolve({})
    })
    apiMocks.put.mockResolvedValue({ state: 'installing' })

    const wrapper = mount(LoggingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], storageClasses: [] } })
    await flushPromises()

    await wrapper.get('[data-testid="logging-settings"]').trigger('click')
    const modal = document.body.querySelector('.logging-settings-modal')
    const retentionInput = modal.querySelector('.settings-retention input')
    retentionInput.value = '21'
    retentionInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    modal.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(apiMocks.put).toHaveBeenCalledWith('/monitoring/logs/config', { node_name: 'node-a', retention_days: 21 })
  })

  it('keeps the previous log results when a later query fails', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'ready', node_name: 'node-a', loki_ready: 1, alloy_ready: 1, alloy_desired: 1 })
      if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: [], nodes: [], pods: [] })
      if (path === '/applications' || path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })
    apiMocks.post
      .mockResolvedValueOnce({ lines: [{ timestamp: '2026-08-05T10:00:00Z', line: '保留的日志', labels: {} }], has_more: false })
      .mockRejectedValueOnce(new Error('Loki 查询超时'))
    const wrapper = mount(LoggingWorkspace)
    await flushPromises()

    await wrapper.get('form.logging-query').trigger('submit')
    await flushPromises()
    await wrapper.get('form.logging-query').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('保留的日志')
    expect(wrapper.text()).toContain('Loki 查询超时')
  })

  it('shows a filter error without hiding an available logging workspace', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'ready', message: '日志采集中', node_name: 'node-a', loki_ready: 1, alloy_ready: 1, alloy_desired: 1 })
      if (path === '/monitoring/logs/filters') return Promise.reject(new Error('筛选项暂时不可用'))
      if (path === '/applications' || path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })
    const wrapper = mount(LoggingWorkspace)
    await flushPromises()

    expect(wrapper.text()).toContain('日志检索')
    expect(wrapper.text()).toContain('筛选项暂时不可用')
  })

  it('keeps the settings drawer open and scopes a failed settings save to it', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/logs/status') return Promise.resolve({ state: 'ready', node_name: 'node-a', loki_ready: 1, alloy_ready: 1, alloy_desired: 1, retention_days: 14 })
      if (path === '/monitoring/logs/filters') return Promise.resolve({ namespaces: [], nodes: [], pods: [] })
      if (path === '/applications' || path === '/projects') return Promise.resolve([])
      return Promise.resolve({})
    })
    apiMocks.put.mockRejectedValue(new Error('保留策略保存失败'))
    const wrapper = mount(LoggingWorkspace)
    await flushPromises()

    await wrapper.get('[data-testid="logging-settings"]').trigger('click')
    const modal = document.body.querySelector('.logging-settings-modal')
    modal.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(document.body.querySelector('.logging-settings-modal')).not.toBeNull()
    expect(modal.textContent).toContain('保留策略保存失败')
    expect(wrapper.text()).toContain('日志检索')
    wrapper.unmount()
  })

  it('aborts a pending logging status read when the workspace unmounts', async () => {
    const pending = deferred()
    apiMocks.get.mockReturnValue(pending.promise)
    const wrapper = mount(LoggingWorkspace)
    await flushPromises()

    const [, options] = apiMocks.get.mock.calls[0]
    wrapper.unmount()
    expect(options.signal.aborted).toBe(true)
  })
})
