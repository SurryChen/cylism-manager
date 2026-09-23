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
    await wrapper.get('[data-testid="open-dns-config"]').trigger('click')
    await wrapper.get('#dns-config-form').trigger('submit')
    await wrapper.get('.base-modal-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/cluster-dns', { resolvers: ['223.5.5.5'] })
  })

  it('splits DNS tables into two cards and moves configuration to a modal', async () => {
    const wrapper = mount(ClusterDNS)
    await flushPromises()

    expect(wrapper.findAll('.dns-workspace')).toHaveLength(2)
    expect(wrapper.find('#dns-config-form').exists()).toBe(false)
    expect(wrapper.find('.dns-runtime-workspace h2').exists()).toBe(false)
    expect(wrapper.find('.dns-history-workspace h2').exists()).toBe(false)
    expect(wrapper.findAll('.dns-history-table th').map(cell => cell.text())).toEqual(['版本', 'DNS 上游', '创建时间', '状态', '操作'])

    await wrapper.get('[data-testid="open-dns-config"]').trigger('click')
    expect(wrapper.get('#dns-config-form').exists()).toBe(true)
    expect(wrapper.get('#dns-config-form').text()).toContain('添加备用上游')
  })

  it('keeps the apply confirmation open and reports a local error on failure', async () => {
    apiMocks.post.mockRejectedValueOnce(new Error('CoreDNS 更新失败'))
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    await wrapper.get('[data-testid="open-dns-config"]').trigger('click')
    await wrapper.get('#dns-config-form').trigger('submit')
    await wrapper.get('.base-modal-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('CoreDNS 更新失败')
  })

  it('retains the last successful data after a failed refresh', async () => {
    const wrapper = mount(ClusterDNS)
    await flushPromises()
    apiMocks.get.mockRejectedValueOnce(new Error('刷新失败'))
    await wrapper.get('[aria-label="刷新 CoreDNS 状态"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('223.5.5.5')
    expect(wrapper.text()).toContain('刷新失败')
    expect(wrapper.get('[role="dialog"]').text()).toContain('刷新失败')
  })

  it('renders the newest refresh result when an older request resolves later', async () => {
    let firstResolve
    let secondResolve
    apiMocks.get
      .mockImplementationOnce(() => new Promise(resolve => { firstResolve = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { secondResolve = resolve }))
    const wrapper = mount(ClusterDNS)
    await wrapper.get('[aria-label="刷新 CoreDNS 状态"]').trigger('click')
    secondResolve({ ...state(), forwarding: ['1.1.1.1'], active_policy: { revision: 1, resolvers: ['1.1.1.1'] } })
    await flushPromises()
    firstResolve({ ...state(), forwarding: ['9.9.9.9'], active_policy: { revision: 1, resolvers: ['9.9.9.9'] } })
    await flushPromises()
    await wrapper.get('[data-testid="open-dns-config"]').trigger('click')
    expect(wrapper.get('[aria-label="DNS 上游 1"]').element.value).toBe('1.1.1.1')
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
