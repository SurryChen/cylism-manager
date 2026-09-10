import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Sites from './Sites.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  api.get.mockImplementation(path => {
    if (path === '/routes') return Promise.resolve([{ namespace: 'default', name: 'console', domain: 'console.example.com' }])
    if (path === '/k8s/ingresses') return Promise.resolve([])
    return Promise.resolve({ type: 'Traefik', running: true })
  })
})

describe('Sites view', () => {
  it('keeps route deletion confirmation open and shows an error when deletion fails', async () => {
    api.delete.mockRejectedValueOnce(new Error('路由仍被引用'))
    const wrapper = mount(Sites)
    await flushPromises()
    await wrapper.get('.btn-danger').trigger('click')
    await wrapper.findAll('.modal-actions .btn-danger').find(button => button.text() === '确认删除').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('路由仍被引用')
    expect(wrapper.text()).toContain('确定删除')
  })
})
