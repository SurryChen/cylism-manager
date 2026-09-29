import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SystemSettingsCloudProviders from './SystemSettingsCloudProviders.vue'

const cloud = vi.hoisted(() => ({
  createCloudConnection: vi.fn(),
  deleteCloudConnection: vi.fn(),
  getCloudConnections: vi.fn(),
  getCloudProviders: vi.fn(),
  getCloudConnectionPermissions: vi.fn(),
  updateCloudConnection: vi.fn(),
  validateCloudConnection: vi.fn(),
}))

vi.mock('../../api/cloud-resources.js', () => cloud)

beforeEach(() => {
  vi.clearAllMocks()
  cloud.getCloudConnections.mockResolvedValue([{ id: 4, name: '生产连接', provider: 'aliyun', dns_enabled: true, object_storage_enabled: true, credential_configured: true }])
  cloud.getCloudProviders.mockResolvedValue([
    { id: 'aliyun', name: '阿里云', description: 'Alibaba Cloud DNS 与 OSS', implemented: true, capabilities: ['dns', 'object_storage'], credential_fields: [{ key: 'access_key_id', label: 'AccessKey ID', type: 'text', required: true }, { key: 'access_key_secret', label: 'AccessKey Secret', type: 'password', required: true }], configuration_fields: [{ key: 'region', label: '默认地域', type: 'text', required: true }] },
    { id: 'tencent', name: '腾讯云', description: 'Tencent Cloud DNSPod 与 COS', implemented: false, capabilities: ['dns', 'object_storage'], credential_fields: [], configuration_fields: [] },
    { id: 'cloudcone', name: 'CloudCone', description: 'CloudCone API 与 S3 兼容对象存储', implemented: false, capabilities: ['object_storage'], credential_fields: [], configuration_fields: [] },
  ])
  cloud.createCloudConnection.mockResolvedValue({})
  cloud.getCloudConnectionPermissions.mockResolvedValue({ status: 'complete', inspected_at: '2026-09-29T08:00:00Z', identity: { type: 'RAMUser', arn: 'acs:ram::123:user/cylism' }, policies: [
    { name: 'AliyunDNSFullAccess', type: 'System', source: 'direct' },
    { name: 'AliyunOSSFullAccess', type: 'System', source: 'group', group_name: 'cloud-operators' },
  ] })
})

describe('SystemSettingsCloudProviders', () => {
  it('renders connections as settings rows and never renders saved credentials', async () => {
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()

    const row = wrapper.get('[data-testid=cloud-provider-card-4]')
    expect(row.text()).toContain('生产连接')
    expect(row.text()).toContain('阿里云')
    expect(row.text()).toContain('已接入')
    expect(wrapper.get('.cloud-provider-table-header').text()).toContain('连接名')
    expect(wrapper.get('.cloud-provider-table-header').text()).toContain('操作')
    expect(wrapper.text()).not.toContain('服务能力')
    expect(wrapper.text()).not.toContain('access_key_secret')
  })

  it('creates generic connections from the create dialog', async () => {
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()

    await wrapper.get('[data-testid=cloud-provider-open-create]').trigger('click')
    const fields = wrapper.findAll('input.form-input')
    await fields[0].setValue('测试连接')
    await wrapper.get('#cloud-provider-credential-access_key_id').setValue('id')
    await wrapper.get('#cloud-provider-credential-access_key_secret').setValue('secret')
    await wrapper.get('[data-testid=cloud-provider-save]').trigger('click')
    await flushPromises()

    expect(cloud.createCloudConnection).toHaveBeenCalledWith(expect.objectContaining({
      name: '测试连接',
      provider: 'aliyun',
      credentials: '{"access_key_id":"id","access_key_secret":"secret"}',
      configuration: '{"region":"cn-hangzhou"}',
    }))
  })

  it('guides users to add a provider when no connection exists', async () => {
    cloud.getCloudConnections.mockResolvedValue([])
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()

    expect(wrapper.text()).toContain('还没有云提供商连接')
  })

  it('shows the caller and direct and inherited policies on demand', async () => {
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    await flushPromises()

    expect(cloud.getCloudConnectionPermissions).toHaveBeenCalledWith(4, expect.anything())
    expect(wrapper.text()).toContain('acs:ram::123:user/cylism')
    expect(wrapper.text()).toContain('AliyunDNSFullAccess')
    expect(wrapper.text()).toContain('cloud-operators')
    expect(wrapper.text()).toContain('不等于实际有效权限')
    expect(wrapper.text()).not.toContain('AccessKey Secret')
  })

  it('distinguishes unavailable inspection from an empty policy list', async () => {
    cloud.getCloudConnectionPermissions.mockResolvedValue({ status: 'unavailable', message: '缺少 RAM 查询权限', policies: [] })
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('缺少 RAM 查询权限')
    expect(wrapper.text()).not.toContain('未附加策略')
  })

  it('shows an empty result only after a complete inspection', async () => {
    cloud.getCloudConnectionPermissions.mockResolvedValue({ status: 'complete', policies: [] })
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('未附加策略')
    expect(wrapper.text()).toContain('不等于实际有效权限')
  })

  it('ignores a closed inspection request when the dialog is reopened', async () => {
    let resolveFirst
    cloud.getCloudConnectionPermissions
      .mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockResolvedValueOnce({ status: 'partial', message: '用户组策略未读取', policies: [] })
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    expect(wrapper.text()).toContain('正在读取 RAM 授权策略')

    const close = wrapper.findAll('button').find(button => button.text() === '关闭')
    expect(close).toBeDefined()
    await close.trigger('click')
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    await flushPromises()
    resolveFirst({ status: 'complete', policies: [{ name: 'stale-policy', source: 'direct' }] })
    await flushPromises()

    expect(wrapper.text()).toContain('用户组策略未读取')
    expect(wrapper.text()).not.toContain('stale-policy')
    expect(cloud.getCloudConnectionPermissions).toHaveBeenCalledTimes(2)
  })

  it('retries a failed inspection from the dialog', async () => {
    cloud.getCloudConnectionPermissions
      .mockRejectedValueOnce(new Error('读取失败'))
      .mockResolvedValueOnce({ status: 'complete', policies: [] })
    const wrapper = mount(SystemSettingsCloudProviders)
    await flushPromises()
    await wrapper.get('[data-testid=cloud-provider-permissions-4]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('读取失败')

    const refresh = wrapper.findAll('button').find(button => button.text() === '刷新')
    await refresh.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('未附加策略')
    expect(wrapper.text()).not.toContain('读取失败')
  })
})
