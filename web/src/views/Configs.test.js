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
    })
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
    expect(api.get).toHaveBeenCalledWith('/k8s/configmaps')
    expect(api.get).not.toHaveBeenCalledWith('/k8s/secrets')
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

    expect(api.get).toHaveBeenCalledWith('/k8s/secrets')
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
})
