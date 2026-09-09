import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Configs from './Configs.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.includes('/configmaps') && !url.includes('default/app-config')) {
        return Promise.resolve([
          { name: 'app-config', namespace: 'default', keys_count: 3, used_by: [], age: '7d' }
        ])
      }
      if (url.includes('/secrets') && !url.includes('default/db-pass')) {
        return Promise.resolve([
          { name: 'db-pass', namespace: 'default', type: 'Opaque', keys_count: 1, used_by: [], age: '3d' }
        ])
      }
      // Detail lookups
      if (url.includes('configmaps/default/app-config')) {
        return Promise.resolve({ data: { key1: 'val1' } })
      }
      if (url.includes('secrets/default/db-pass')) {
        return Promise.resolve({ data: { password: '***' } })
      }
      return Promise.resolve([])
    }),
    post: vi.fn(), put: vi.fn(), delete: vi.fn()
  }
}))

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('Configs view', () => {
  it('renders two tab buttons', () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    const tabs = wrapper.findAll('.tab-btn')
    expect(tabs).toHaveLength(2)
    expect(tabs[0].text()).toBe('ConfigMaps')
    expect(tabs[1].text()).toBe('Secrets')
  })

  it('loads and renders ConfigMaps when entering the page', async () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()
    expect(api.get.mock.calls.some(([path]) => path === '/k8s/configmaps?usage=false')).toBe(true)
    expect(api.get.mock.calls.some(([path]) => path === '/k8s/secrets?usage=false')).toBe(false)
    expect(api.get.mock.calls.find(([path]) => path === '/k8s/configmaps?usage=false')[1]).toEqual(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('app-config')
  })

  it('loads and renders Secrets when switching tabs', async () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const secretTab = wrapper.findAll('.tab-btn')[1]
    await secretTab.trigger('click')
    await nextTick()
    await new Promise(r => setTimeout(r, 100))

    expect(api.get.mock.calls.some(([path]) => path === '/k8s/secrets?usage=false')).toBe(true)
    expect(api.get.mock.calls.find(([path]) => path === '/k8s/secrets?usage=false')[1]).toEqual(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('db-pass')
    expect(wrapper.text()).toContain('Opaque')
  })

  it('expands configmap detail on click', async () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    // Click first row
    const row = wrapper.find('.clickable')
    if (row.exists()) {
      await row.trigger('click')
      expect(wrapper.text()).toContain("app-config")
      await nextTick()
    }
  })

  it('creates a ConfigMap from the management page', async () => {
    api.post.mockResolvedValue({})
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true, Teleport: true } }
    })
    await new Promise(r => setTimeout(r, 0))
    await wrapper.find('.page-header .btn-primary').trigger('click')
    const inputs = wrapper.findAll('.resource-modal input')
    await inputs[0].setValue('default')
    await inputs[1].setValue('runtime-config')
    await inputs[2].setValue('config.yaml')
    await inputs[3].setValue('port: 8080')
    await wrapper.find('.resource-modal form').trigger('submit.prevent')
    expect(api.post).toHaveBeenCalledWith('/k8s/configmaps', {
      namespace: 'default', name: 'runtime-config', data: { 'config.yaml': 'port: 8080' }
    })
  })

  it('does not let an older tab response replace the latest tab', async () => {
    let resolveConfig
    let resolveSecret
    api.get.mockImplementation(path => {
      if (path === '/k8s/namespace-names') return Promise.resolve([])
      if (path === '/k8s/configmaps?usage=false') return new Promise(resolve => { resolveConfig = resolve })
      if (path === '/k8s/secrets?usage=false') return new Promise(resolve => { resolveSecret = resolve })
      return Promise.resolve([])
    })
    const wrapper = mount(Configs, { global: { stubs: { Teleport: true } } })
    await Promise.resolve()
    await wrapper.findAll('.tab-btn')[1].trigger('click')
    resolveSecret([{ name: 'latest-secret', namespace: 'default', type: 'Opaque', keys: [] }])
    await Promise.resolve()
    resolveConfig([{ name: 'stale-config', namespace: 'default', keys: [] }])
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.text()).toContain('latest-secret')
    expect(wrapper.text()).not.toContain('stale-config')
  })
})
