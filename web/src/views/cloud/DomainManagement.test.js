import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DomainManagement from './DomainManagement.vue'

const cloud = vi.hoisted(() => ({
  createCloudDNSRecord: vi.fn(),
  deleteCloudDNSRecord: vi.fn(),
  getCloudConnections: vi.fn(),
  getCloudDNSRecords: vi.fn(),
  getCloudZones: vi.fn(),
  updateCloudDNSRecord: vi.fn(),
}))

vi.mock('../../api/cloud-resources.js', () => cloud)

beforeEach(() => {
  vi.clearAllMocks()
  cloud.getCloudConnections.mockResolvedValue([{ id: 7, name: '生产', provider: 'aliyun' }])
  cloud.getCloudZones.mockResolvedValue([{ name: 'example.com' }])
  cloud.getCloudDNSRecords.mockResolvedValue({ records: [{ id: 'record-1', rr: '@', type: 'A', line: 'default', value: '1.2.3.4', ttl: 600 }] })
  cloud.createCloudDNSRecord.mockResolvedValue({})
  cloud.updateCloudDNSRecord.mockResolvedValue({})
  cloud.deleteCloudDNSRecord.mockResolvedValue({})
})

describe('DomainManagement', () => {
  it('manages DNS records from the dedicated page', async () => {
    const wrapper = mount(DomainManagement)
    await flushPromises()

    await wrapper.get('[data-testid=domain-record-create]').trigger('click')
    await wrapper.get('[data-testid=domain-record-value]').setValue('5.6.7.8')
    await wrapper.get('[data-testid=domain-record-save]').trigger('click')
    await flushPromises()
    expect(cloud.createCloudDNSRecord).toHaveBeenCalledWith(7, expect.objectContaining({ zone: 'example.com', value: '5.6.7.8' }))

    vi.stubGlobal('confirm', vi.fn(() => true))
    await wrapper.get('[data-testid=domain-record-delete-record-1]').trigger('click')
    await flushPromises()
    expect(cloud.deleteCloudDNSRecord).toHaveBeenCalledWith(7, 'record-1', { confirm: true })
  })
})
