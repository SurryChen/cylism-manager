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
  it('offers an installation form with a ready node and hostPath data directory', async () => {
    const wrapper = mount(Monitoring)
    await flushPromises()

    expect(wrapper.text()).toContain('安装 VictoriaMetrics')
    expect(wrapper.find('select').text()).toContain('node-a')
    expect(wrapper.find('input[placeholder="/data/victoria-metrics"]').element.value).toBe('/data/victoria-metrics')
  })
})
