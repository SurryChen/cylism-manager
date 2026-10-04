import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccessCertificates from './AccessCertificates.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const api = vi.hoisted(() => ({
  getAccessCA: vi.fn(), getAccessCertificates: vi.fn(), getClusters: vi.fn(),
  initializeAccessCA: vi.fn(), issueAccessCertificate: vi.fn(), revokeAccessCertificate: vi.fn(), deleteAccessCertificate: vi.fn(),
}))
vi.mock('../../api/access-certificates.js', () => ({
  ...api, getAccessCertificatePEM: vi.fn(), getAccessCRL: vi.fn(),
}))
vi.mock('../../api/clusters.js', () => ({ getClusters: api.getClusters }))

describe('AccessCertificates', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    api.getAccessCA.mockResolvedValue(null)
    api.getAccessCertificates.mockResolvedValue([])
    api.getClusters.mockResolvedValue([{ id: 3, name: '生产集群' }])
  })

  it('creates a CA, issues a cluster-bound certificate, and revokes it', async () => {
    const ca = { certificate_pem: 'CA', not_after: '2099-01-01T00:00:00Z' }
    const cert = { id: 8, cluster_id: 3, kind: 'platform_client', subject: 'client', fingerprint: '1234567890123456', not_after: '2099-01-01T00:00:00Z' }
    api.initializeAccessCA.mockResolvedValue(ca)
    api.issueAccessCertificate.mockResolvedValue(cert)
    api.revokeAccessCertificate.mockResolvedValue(null)
    api.getAccessCA.mockResolvedValueOnce(null).mockResolvedValueOnce(ca)
    api.getAccessCertificates.mockResolvedValueOnce([]).mockResolvedValueOnce([{ ...cert, revoked_at: '2026-10-02T00:00:00Z' }])

    const wrapper = mount(AccessCertificates)
    await flushPromises()
    await wrapper.get('.access-cert-actions .btn-primary').trigger('click')
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(api.initializeAccessCA).toHaveBeenCalledOnce()
    expect(wrapper.get('.access-cert-tooltip[data-tooltip*="Agent 建立信任"]').attributes('data-tooltip')).toContain('Agent 建立信任')
    expect(wrapper.get('.access-cert-tooltip[data-tooltip*="CRL"]').attributes('data-tooltip')).toContain('已吊销')

    await wrapper.get('.access-cert-actions .btn-primary').trigger('click')
    const menus = wrapper.findAllComponents(SelectMenu)
    menus[0].vm.$emit('update:modelValue', '3')
    menus[1].vm.$emit('update:modelValue', 'platform_client')
    await wrapper.get('#issue-access-certificate').trigger('submit')
    await flushPromises()
    expect(api.issueAccessCertificate).toHaveBeenCalledWith({ cluster_id: 3, kind: 'platform_client', csr: '' })
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)

    await wrapper.get('[aria-label="吊销证书 #8"]').trigger('click')
    await wrapper.get('[role="dialog"] .btn-danger').trigger('click')
    await flushPromises()
    expect(api.revokeAccessCertificate).toHaveBeenCalledWith(8)
    expect(wrapper.get('tbody').text()).toContain('已吊销')
  })

  it('requires a cluster before issuing certificates', async () => {
    api.getAccessCA.mockResolvedValue({ certificate_pem: 'CA', not_after: '2099-01-01T00:00:00Z' })
    api.getClusters.mockResolvedValue([])
    const wrapper = mount(AccessCertificates)
    await flushPromises()
    expect(wrapper.get('.access-cert-actions .btn-primary').attributes('disabled')).toBeDefined()
  })

  it('shows usage status and deletes only unused certificates', async () => {
    api.getAccessCA.mockResolvedValue({ certificate_pem: 'CA', not_after: '2099-01-01T00:00:00Z' })
    api.getAccessCertificates.mockResolvedValue([
      { id: 11, cluster_id: 3, kind: 'agent_server', subject: 'agent', fingerprint: '1234567890123456', not_after: '2099-01-01T00:00:00Z', in_use: true },
      { id: 12, cluster_id: 3, kind: 'platform_client', subject: 'client', fingerprint: '6543210987654321', not_after: '2099-01-01T00:00:00Z', in_use: false },
    ])
    api.deleteAccessCertificate.mockResolvedValue(null)
    const wrapper = mount(AccessCertificates)
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('使用中')
    expect(wrapper.get('tbody').text()).toContain('未使用')
    expect(wrapper.find('[aria-label="删除未使用证书 #11"]').exists()).toBe(false)
    await wrapper.get('[aria-label="删除未使用证书 #12"]').trigger('click')
    await wrapper.get('[role="dialog"] .btn-danger').trigger('click')
    await flushPromises()
    expect(api.deleteAccessCertificate).toHaveBeenCalledWith(12)
  })
})
