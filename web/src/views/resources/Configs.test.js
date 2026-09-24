import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Configs from './Configs.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url === '/k8s/namespace-names') return Promise.resolve([])
      if (url === '/k8s/configmaps') return Promise.resolve([{ name: 'app-config', namespace: 'default', keys_count: 3, used_by: [{ kind: 'Deployment', name: 'web', namespace: 'default' }], age: '7d' }])
      if (url === '/k8s/secrets') return Promise.resolve([{ name: 'db-pass', namespace: 'default', type: 'Opaque', keys_count: 1, keys: ['password'], used_by: [], age: '3d' }])
      if (url === '/k8s/configmaps/default/app-config') return Promise.resolve({ data: { key1: 'val1' } })
      return Promise.resolve([])
    }),
    post: vi.fn(), put: vi.fn(), delete: vi.fn()
  }
}))

beforeEach(() => { vi.clearAllMocks(); document.body.innerHTML = '' })

async function mountLoaded() {
  const wrapper = mount(Configs)
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
  return wrapper
}

describe('Configs view', () => {
  it('uses one card toolbar without a repeated page title', async () => {
    const wrapper = await mountLoaded()
    expect(wrapper.find('.page-title').exists()).toBe(false)
    expect(wrapper.findAll('.tab-btn')).toHaveLength(2)
    const createButton = wrapper.get('[data-testid="create-config-resource"]')
    expect(createButton.classes()).toContain('btn')
    expect(createButton.classes()).toContain('btn-primary')
    expect(createButton.classes()).not.toContain('btn-sm')
  })

  it('loads ConfigMaps with reference metadata on mount', async () => {
    const wrapper = await mountLoaded()
    expect(api.get).toHaveBeenCalledWith('/k8s/configmaps', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('键数量')
    expect(wrapper.text()).toContain('引用数量')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('1')
  })

  it('loads Secrets when switching tabs', async () => {
    const wrapper = await mountLoaded()
    await wrapper.findAll('.tab-btn')[1].trigger('click')
    await Promise.resolve()
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/k8s/secrets', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('db-pass')
  })

  it('shows ConfigMap data and references in a detail modal', async () => {
    const wrapper = await mountLoaded()
    await wrapper.find('[data-testid="view-config-resource-default/app-config"]').trigger('click')
    await Promise.resolve()
    await nextTick()
    expect(wrapper.text()).toContain('ConfigMap · default/app-config')
    expect(wrapper.text()).toContain('key1')
    expect(wrapper.text()).toContain('引用工作负载')
  })
})
