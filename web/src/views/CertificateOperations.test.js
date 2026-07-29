import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CertificateOperations from './CertificateOperations.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn() } }))
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { namespace: 'production', name: 'api-cert' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('CertificateOperations view', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows the DNS challenge failure returned by cert-manager', async () => {
    api.get.mockResolvedValue([{ kind: 'Challenge', name: 'api-cert-1', domain: 'api.example.com', type: 'DNS-01', status: 'Failed', reason: 'RecordNotFound', created_at: '2026-07-29 10:00' }])
    const wrapper = mount(CertificateOperations)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.get).toHaveBeenCalledWith('/certs/production/api-cert/operations')
    expect(wrapper.text()).toContain('RecordNotFound')
    expect(wrapper.text()).toContain('DNS-01')
  })
})
