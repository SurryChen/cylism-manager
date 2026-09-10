import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import CertificateOperations from './CertificateOperations.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn() } }))
const routeParams = reactive({ namespace: 'production', name: 'api-cert' })
let mountedWrapper
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: routeParams }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('CertificateOperations view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockReset()
    routeParams.namespace = 'production'
    routeParams.name = 'api-cert'
  })
  afterEach(() => mountedWrapper?.unmount())

  it('shows the DNS challenge failure returned by cert-manager', async () => {
    api.get.mockResolvedValue([{ kind: 'Challenge', name: 'api-cert-1', domain: 'api.example.com', type: 'DNS-01', status: 'Failed', reason: 'RecordNotFound', created_at: '2026-07-29 10:00' }])
    const wrapper = mountedWrapper = mount(CertificateOperations)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.get).toHaveBeenCalledWith('/certs/production/api-cert/operations', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('RecordNotFound')
    expect(wrapper.text()).toContain('DNS-01')
  })

  it('commits only the latest route response', async () => {
    let resolveFirst
    let resolveSecond
    api.get
      .mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { resolveSecond = resolve }))
    const wrapper = mountedWrapper = mount(CertificateOperations)
    await nextTick()
    routeParams.name = 'web-cert'
    await nextTick()
    await nextTick()
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
    resolveSecond([{ kind: 'CertificateRequest', name: 'web-cert-2', status: 'Valid' }])
    await flushPromises()
    resolveFirst([{ kind: 'Challenge', name: 'stale', status: 'Failed' }])
    await flushPromises()

    expect(wrapper.text()).toContain('web-cert-2')
    expect(wrapper.text()).not.toContain('stale')
  })

  it('does not show an abort error during route navigation', async () => {
    const pending = new Promise(() => {})
    let requestOptions
    api.get.mockImplementation((_path, options) => {
      requestOptions = options
      return pending
    })
    const wrapper = mountedWrapper = mount(CertificateOperations)
    await nextTick()
    routeParams.namespace = 'production'
    routeParams.name = 'another-cert'
    await nextTick()
    expect(wrapper.text()).not.toContain('AbortError')
    wrapper.unmount()
    expect(requestOptions.signal.aborted).toBe(true)
  })
})
