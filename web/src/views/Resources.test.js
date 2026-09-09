import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Resources from './Resources.vue'
import { getResourceInventory } from '../api/kubernetes.js'

vi.mock('../api/kubernetes.js', () => ({ getResourceInventory: vi.fn(), getResourceService: vi.fn() }))

const inventory = namespace => Promise.resolve([
  [{ name: `pod-${namespace}`, namespace, status: 'Running' }],
  [{ name: `service-${namespace}`, namespace, type: 'ClusterIP', ports: ['80/TCP'] }],
  [{ name: `deployment-${namespace}`, namespace, replicas: 1, ready: 1, images: ['example:latest'] }],
])

describe('Resources view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getResourceInventory.mockImplementation((namespace = '') => inventory(namespace || 'all'))
  })

  afterEach(() => vi.restoreAllMocks())

  it('loads the inventory through the managed domain API', async () => {
    const wrapper = mount(Resources)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(getResourceInventory).toHaveBeenCalledWith('', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('pod-all')
  })

  it('does not render an older namespace response after a newer filter is selected', async () => {
    let resolveFirst
    getResourceInventory.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockImplementationOnce(() => inventory('new'))
    const wrapper = mount(Resources)
    wrapper.vm.filterNs = 'new'
    await new Promise(resolve => setTimeout(resolve, 0))
    resolveFirst(await inventory('old'))
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(wrapper.text()).toContain('pod-new')
    expect(wrapper.text()).not.toContain('pod-old')
    wrapper.unmount()
  })

  it('cancels an inventory request when the view is unmounted', async () => {
    let requestSignal
    getResourceInventory.mockImplementation((_namespace, options) => {
      requestSignal = options.signal
      return new Promise(() => {})
    })
    const wrapper = mount(Resources)
    await nextTick()

    wrapper.unmount()

    expect(requestSignal.aborted).toBe(true)
  })

  it('keeps the resource page visible and shows a local read error', async () => {
    getResourceInventory.mockRejectedValueOnce(new Error('集群资源不可用'))
    const wrapper = mount(Resources)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(wrapper.find('.page-title').text()).toBe('资源浏览')
    expect(wrapper.text()).toContain('集群资源不可用')
  })
})
