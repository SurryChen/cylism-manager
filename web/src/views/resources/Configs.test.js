import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Configs from './Configs.vue'
import { api } from '../../api/index.js'

function defaultGet(url) {
  if (url === '/k8s/namespace-names') return Promise.resolve([])
  if (url === '/k8s/configmaps?namespace=&usage=false') return Promise.resolve([{ name: 'app-config', namespace: 'default', keys_count: 3, age: '7d' }])
  if (url === '/k8s/secrets?namespace=&metadata=true&limit=50') return Promise.resolve({ items: [{ name: 'db-pass', namespace: 'default', age: '3d' }], continue: '' })
  if (url === '/k8s/secrets?namespace=&metadata=true&limit=50&continue=next-page') return Promise.resolve({ items: [{ name: 'api-key', namespace: 'default', age: '2d' }], continue: '' })
  if (url === '/k8s/configmaps/default/app-config') return Promise.resolve({ data: { key1: 'val1' }, used_by: [{ kind: 'Deployment', name: 'web', namespace: 'default' }] })
  if (url === '/k8s/secrets/default/db-pass') return Promise.resolve({ data: { password: '[REDACTED]' }, used_by: [{ kind: 'StatefulSet', name: 'database', namespace: 'default' }] })
  return Promise.resolve([])
}

vi.mock('../../api/index.js', () => ({ api: { get: vi.fn(defaultGet), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

beforeEach(() => { vi.clearAllMocks(); api.get.mockImplementation(defaultGet); document.body.innerHTML = '' })

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

  it('loads only lightweight ConfigMap metadata on mount', async () => {
    const wrapper = await mountLoaded()
    expect(api.get).toHaveBeenCalledWith('/k8s/configmaps?namespace=&usage=false', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(api.get).not.toHaveBeenCalledWith('/k8s/namespace-names')
    expect(wrapper.text()).toContain('键数量')
    expect(wrapper.text()).not.toContain('引用数量')
    expect(wrapper.text()).toContain('3')
  })

  it('loads Secrets when switching tabs', async () => {
    const wrapper = await mountLoaded()
    await wrapper.findAll('.tab-btn')[1].trigger('click')
    await Promise.resolve()
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/k8s/secrets?namespace=&metadata=true&limit=50', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('db-pass')
  })

  it('appends the next metadata page without reloading the first page', async () => {
    api.get.mockImplementation(url => {
      if (url === '/k8s/configmaps?namespace=&usage=false') return Promise.resolve([])
      if (url === '/k8s/namespace-names') return Promise.resolve([])
      if (url === '/k8s/secrets?namespace=&metadata=true&limit=50') return Promise.resolve({ items: [{ name: 'db-pass', namespace: 'default', age: '3d' }], continue: 'next-page' })
      if (url === '/k8s/secrets?namespace=&metadata=true&limit=50&continue=next-page') return Promise.resolve({ items: [{ name: 'api-key', namespace: 'default', age: '2d' }], continue: '' })
      return Promise.resolve([])
    })
    const wrapper = await mountLoaded()
    await wrapper.findAll('.tab-btn')[1].trigger('click')
    await Promise.resolve()
    await nextTick()
    await wrapper.get('.secret-pagination button').trigger('click')
    await Promise.resolve()
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/k8s/secrets?namespace=&metadata=true&limit=50&continue=next-page', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('db-pass')
    expect(wrapper.text()).toContain('api-key')
  })

  it('shows ConfigMap data and references in a detail modal', async () => {
    const wrapper = await mountLoaded()
    await wrapper.find('[data-testid="view-config-resource-default/app-config"]').trigger('click')
    await Promise.resolve()
    await nextTick()
    expect(wrapper.text()).toContain('ConfigMap · default/app-config')
    expect(wrapper.text()).toContain('key1')
    expect(wrapper.text()).toContain('引用工作负载')
    expect(wrapper.text()).toContain('web')
  })

  it('loads Secret keys and references only when details are opened', async () => {
    const wrapper = await mountLoaded()
    await wrapper.findAll('.tab-btn')[1].trigger('click')
    await Promise.resolve()
    await nextTick()

    await wrapper.find('[data-testid="view-config-resource-default/db-pass"]').trigger('click')
    await Promise.resolve()
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/k8s/secrets/default/db-pass', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('StatefulSet')
    expect(wrapper.text()).toContain('database')
  })

  it('loads namespace options when the create form opens', async () => {
    const wrapper = await mountLoaded()
    expect(api.get).not.toHaveBeenCalledWith('/k8s/namespace-names')

    await wrapper.get('[data-testid="create-config-resource"]').trigger('click')
    await Promise.resolve()
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/k8s/namespace-names')
  })
})
