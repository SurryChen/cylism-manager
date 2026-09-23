import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Sites from './Sites.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() } }))

const ingresses = [
  { namespace: 'default', name: 'console', hosts: ['console.example.com'], paths: ['/ -> console:8080'], tls: ['console-tls'], controller: 'traefik', age: '2d' },
  { namespace: 'production', name: 'api', hosts: ['api.example.com'], paths: ['/ -> api:8080'], tls: [], controller: 'traefik', age: '1d' },
]

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  api.get.mockImplementation(path => {
    if (path === '/k8s/ingresses') return Promise.resolve(ingresses)
    return Promise.resolve({ type: 'Traefik', version: '3.7.4', running: true })
  })
})

describe('Sites view', () => {
  it('only renders standard Ingress records and does not load IngressRoute data', async () => {
    const wrapper = mount(Sites)
    await flushPromises()

    expect(wrapper.text()).toContain('console.example.com')
    expect(wrapper.text()).not.toContain('IngressRoute')
    expect(api.get).not.toHaveBeenCalledWith('/routes', expect.anything())
  })

  it('filters standard Ingress records by namespace', async () => {
    const wrapper = mount(Sites)
    await flushPromises()
    await wrapper.get('[data-testid="ingress-namespace-filter"]').setValue('production')

    expect(wrapper.text()).toContain('api.example.com')
    expect(wrapper.text()).not.toContain('console.example.com')
  })

  it('keeps the deletion confirmation open and shows an error when deletion fails', async () => {
    api.delete.mockRejectedValueOnce(new Error('Ingress 仍被引用'))
    const wrapper = mount(Sites)
    await flushPromises()
    await wrapper.get('.btn-danger').trigger('click')
    await wrapper.findAll('.modal-actions .btn-danger').find(button => button.text() === '确认删除').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Ingress 仍被引用')
    expect(wrapper.text()).toContain('确定删除')
  })
})
