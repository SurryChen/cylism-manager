import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DiskGrowthWorkspace from './DiskGrowthWorkspace.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn() }))
function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => { resolve = resolvePromise; reject = rejectPromise })
  return { promise, resolve, reject }
}

vi.mock('../api/index.js', () => ({ api: apiMocks }))

beforeEach(() => {
  apiMocks.get.mockReset()
  apiMocks.get.mockResolvedValue({
    mounts: [{ node: 'node-a', mount_point: '/var/lib', growth_bytes: 1048576 }],
    pvcs: [{ node: 'node-a', namespace: 'project-demo', pvc: 'data', growth_bytes: 2097152, consumers: ['app-1'] }],
  })
})

describe('DiskGrowthWorkspace', () => {
  it('loads diagnostics on mount and reloads with node and range filters', async () => {
    const wrapper = mount(DiskGrowthWorkspace, { props: { nodes: [{ name: 'node-a', display_name: '主节点', ready: true }] } })
    await flushPromises()

    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=6h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('/var/lib')
    expect(wrapper.text()).toContain('使用者: app-1')
    expect(wrapper.text()).not.toContain('容器可写层')

    const selects = wrapper.findAll('select')
    await selects[0].setValue('24h')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    await selects[1].setValue('node-a')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=24h&node=node-a', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('shows a warning while retaining available diagnostic groups', async () => {
    apiMocks.get.mockResolvedValue({
      mounts: [{ node: 'node-a', mount_point: '/var/lib', growth_bytes: 1048576 }],
      pvcs: [],
      warnings: { pvcs: 'VictoriaMetrics 返回 422 Unprocessable Entity' },
    })
    const wrapper = mount(DiskGrowthWorkspace)
    await flushPromises()

    expect(wrapper.text()).toContain('PVC 查询失败：VictoriaMetrics 返回 422 Unprocessable Entity')
    expect(wrapper.text()).toContain('/var/lib')
  })

  it('keeps only the most recent filter result when requests resolve out of order', async () => {
    const initial = deferred()
    const older = deferred()
    const current = deferred()
    apiMocks.get.mockImplementation(path => {
      if (path.includes('range=6h')) return initial.promise
      if (path.includes('range=24h')) return older.promise
      return current.promise
    })
    const wrapper = mount(DiskGrowthWorkspace)
    initial.resolve({ mounts: [{ mount_point: '/initial' }], pvcs: [] })
    await flushPromises()

    const [rangeSelect] = wrapper.findAll('select')
    await rangeSelect.setValue('24h')
    await rangeSelect.setValue('7d')
    current.resolve({ mounts: [{ mount_point: '/latest' }], pvcs: [] })
    await flushPromises()
    older.resolve({ mounts: [{ mount_point: '/stale' }], pvcs: [] })
    await flushPromises()

    expect(wrapper.text()).toContain('/latest')
    expect(wrapper.text()).not.toContain('/stale')
  })

  it('aborts a pending disk-growth read when the workspace unmounts', async () => {
    const pending = deferred()
    apiMocks.get.mockReturnValue(pending.promise)
    const wrapper = mount(DiskGrowthWorkspace)
    await flushPromises()

    const [, options] = apiMocks.get.mock.calls[0]
    wrapper.unmount()
    expect(options.signal.aborted).toBe(true)
  })

  it('retains the last diagnostics after a refresh failure', async () => {
    apiMocks.get
      .mockResolvedValueOnce({ mounts: [{ mount_point: '/kept' }], pvcs: [] })
      .mockRejectedValueOnce(new Error('VictoriaMetrics 暂时不可用'))
    const wrapper = mount(DiskGrowthWorkspace)
    await flushPromises()

    await wrapper.findAll('select')[0].setValue('24h')
    await flushPromises()

    expect(wrapper.text()).toContain('/kept')
    expect(wrapper.text()).toContain('VictoriaMetrics 暂时不可用')
  })
})
