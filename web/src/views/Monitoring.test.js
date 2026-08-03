import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Monitoring from './Monitoring.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), delete: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))

beforeEach(() => {
  apiMocks.get.mockReset()
  apiMocks.post.mockReset()
  apiMocks.get.mockImplementation(path => {
    if (path === '/monitoring/status') return Promise.resolve({ state: 'not_installed', message: '尚未安装 VictoriaMetrics' })
    if (path === '/nodes') return Promise.resolve([{ name: 'node-a', ready: true }])
    return Promise.resolve({})
  })
})

describe('Monitoring view', () => {
  it('uses the shared section title bar', () => {
    const wrapper = mount(Monitoring)
    expect(wrapper.get('.section-page-header').find('h1').text()).toBe('集群监控')
    expect(wrapper.find('.section-page-header .page-subtitle').exists()).toBe(false)
  })

  it('offers an installation form with a ready node and hostPath data directory', async () => {
    const wrapper = mount(Monitoring)
    await flushPromises()

    expect(wrapper.text()).toContain('VictoriaMetrics 未安装')
    expect(wrapper.find('.monitoring-install-card').exists()).toBe(true)
    expect(wrapper.text()).toContain('重新检测')
    expect(wrapper.find('select').text()).toContain('node-a')
    expect(wrapper.find('input[placeholder="/data/victoria-metrics"]').element.value).toBe('/data/victoria-metrics')
  })
})
