import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DomainManagement from './DomainManagement.vue'
import SelectMenu from '../../components/SelectMenu.vue'

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
  it('uses the shared tabbed page header for its DNS workspace', async () => {
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.get('[data-header-variant="tabbed"] .page-title').text()).toBe('域名管理')
    expect(wrapper.findAll('.section-tab')).toHaveLength(1)
    expect(wrapper.get('[data-testid="domain-management-tab-records"]').text()).toBe('DNS 记录')
    expect(wrapper.get('[data-testid="domain-management-tab-records"]').classes()).toContain('is-active')
  })

  it('manages DNS records from the dedicated page', async () => {
    const wrapper = mount(DomainManagement)
    await flushPromises()

    await wrapper.get('[data-testid=domain-record-create]').trigger('click')
    await wrapper.get('[data-testid=domain-record-value]').setValue('5.6.7.8')
    await wrapper.get('#domain-record-form').trigger('submit')
    await flushPromises()
    expect(cloud.createCloudDNSRecord).toHaveBeenCalledWith(7, expect.objectContaining({ zone: 'example.com', value: '5.6.7.8' }))

    vi.stubGlobal('confirm', vi.fn(() => true))
    await wrapper.get('[data-testid=domain-record-delete-record-1]').trigger('click')
    await flushPromises()
    expect(cloud.deleteCloudDNSRecord).toHaveBeenCalledWith(7, 'record-1', { confirm: true })
  })

  it('uses shared selectors and places the create action in the record list toolbar', async () => {
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.findAllComponents(SelectMenu)).toHaveLength(2)
    const card = wrapper.get('[data-testid="domain-record-list"]')
    expect(card.get('[data-testid="domain-record-toolbar"] [data-testid="domain-record-create"]')).toBeTruthy()
    expect(card.get('.record-filters .select-menu-trigger').text()).toContain('生产 · 阿里云')
    expect(card.get('table.data-table').text()).toContain('1.2.3.4')

    await wrapper.get('[data-testid="domain-record-create"]').trigger('click')
    expect(wrapper.findAllComponents(SelectMenu)).toHaveLength(3)
  })

  it('shows a safe error dialog and retry state when DNS records cannot be read', async () => {
    const rawError = 'Post "http://alidns.aliyuncs.com/?AccessKeyId=secret&Signature=signed": dial tcp: lookup alidns.aliyuncs.com: i/o timeout'
    cloud.getCloudDNSRecords.mockRejectedValueOnce(new Error(rawError))
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.get('[role="dialog"]').text()).toContain('连接阿里云 DNS 接口超时')
    expect(wrapper.text()).not.toContain('AccessKeyId')
    expect(wrapper.text()).not.toContain('Signature=')
    expect(wrapper.text()).not.toContain('暂无 DNS 记录')
    expect(wrapper.get('[data-testid="domain-record-error-state"]').text()).toContain('DNS 记录暂不可展示')

    await wrapper.get('[data-testid="dismiss-error-notice"]').trigger('click')
    await wrapper.get('[data-testid="domain-record-retry"]').trigger('click')
    await flushPromises()
    expect(cloud.getCloudDNSRecords).toHaveBeenCalledTimes(2)
    expect(wrapper.get('table.data-table').text()).toContain('1.2.3.4')
  })

  it('names the selected provider in timeout errors', async () => {
    cloud.getCloudConnections.mockResolvedValue([{ id: 8, name: '测试', provider: 'tencent' }])
    cloud.getCloudDNSRecords.mockRejectedValueOnce(new Error('dial tcp: lookup dnspod.tencentcloudapi.com: i/o timeout'))
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.get('[role="dialog"]').text()).toContain('腾讯云 DNS 接口超时')
  })

  it('does not repeat the provider when the connection has the same name', async () => {
    cloud.getCloudConnections.mockResolvedValue([{ id: 7, name: '阿里云', provider: 'aliyun' }])
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.get('.record-filters .select-menu-trigger').text()).toBe('阿里云')
  })

  it('shows the shared empty state instead of a horizontally clipped empty table', async () => {
    cloud.getCloudDNSRecords.mockResolvedValueOnce({ records: [] })
    const wrapper = mount(DomainManagement)
    await flushPromises()

    expect(wrapper.get('.record-list .empty-state').text()).toBe('暂无 DNS 记录')
    expect(wrapper.find('table.data-table').exists()).toBe(false)
  })
})
