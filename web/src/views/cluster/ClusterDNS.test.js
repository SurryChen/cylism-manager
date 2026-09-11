import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ClusterDNS from './ClusterDNS.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), delete: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: apiMocks }))

const state = () => ({
  forwarding: ['223.5.5.5'],
  pods: [{ name: 'coredns-1', node: 'node-a', ip: '10.0.0.2', ready: true }],
  history: [{ revision: 1, resolvers: ['223.5.5.5'] }],
  active_policy: { revision: 1, resolvers: ['223.5.5.5'] },
})

describe('ClusterDNS view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockResolvedValue(state())
    apiMocks.post.mockResolvedValue(state())
    apiMocks.delete.mockResolvedValue(state())
  })

  it('loads DNS state and applies through the domain API', async () => {
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    expect(wrapper.text()).toContain('223.5.5.5')
    await wrapper.get('form').trigger('submit')
    await wrapper.get('.modal-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/cluster-dns', { resolvers: ['223.5.5.5'] })
  })

  it('keeps the apply confirmation open and reports a local error on failure', async () => {
    apiMocks.post.mockRejectedValueOnce(new Error('CoreDNS 更新失败'))
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('.modal-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('CoreDNS 更新失败')
  })

  it('retains the last successful data after a failed refresh', async () => {
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    apiMocks.get.mockRejectedValueOnce(new Error('刷新失败'))
    await wrapper.get('.page-header .btn').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('223.5.5.5')
    expect(wrapper.text()).toContain('刷新失败')
  })

  it('renders the newest refresh result when an older request resolves later', async () => {
    let firstResolve
    let secondResolve
    apiMocks.get
      .mockImplementationOnce(() => new Promise(resolve => { firstResolve = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { secondResolve = resolve }))
    const wrapper = mount(ClusterDNS)
    await wrapper.get('.page-header .btn').trigger('click')
    secondResolve({ ...state(), forwarding: ['1.1.1.1'] })
    await flushPromises()
    firstResolve({ ...state(), forwarding: ['9.9.9.9'] })
    await flushPromises()
    expect(wrapper.text()).toContain('1.1.1.1')
    expect(wrapper.text()).not.toContain('9.9.9.9')
  })

  it('aborts the active read when unmounted', async () => {
    let capturedSignal
    apiMocks.get.mockImplementationOnce((_path, options) => {
      capturedSignal = options.signal
      return new Promise(() => {})
    })
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    expect(capturedSignal.aborted).toBe(false)
    wrapper.unmount()
    expect(capturedSignal.aborted).toBe(true)
  })
})
