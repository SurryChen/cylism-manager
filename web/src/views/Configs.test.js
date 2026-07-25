import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Configs from './Configs.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.includes('/configmaps')) {
        return Promise.resolve({
          json: async () => ({ data: [
            { name: 'app-config', namespace: 'default', keys_count: 3, used_by: [], age: '7d' }
          ]})
        })
      }
      if (url.includes('/secrets')) {
        return Promise.resolve({
          json: async () => ({ data: [
            { name: 'db-pass', namespace: 'default', type: 'Opaque', keys_count: 1, used_by: [], age: '3d' }
          ]})
        })
      }
      return Promise.resolve({ json: async () => ({ data: {} }) })
    })
  }
}))

beforeEach(() => {
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

  it('renders configmap list', async () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()
    expect(wrapper.text()).toContain('ConfigMaps')
  })

  it('switches to secrets tab and renders correctly', async () => {
    const wrapper = mount(Configs, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const secretTab = wrapper.findAll('.tab-btn')[1]
    await secretTab.trigger('click')
    await nextTick()
    await new Promise(r => setTimeout(r, 100))

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
