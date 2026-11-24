import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseMonitor from './DatabaseMonitor.vue'
const api = vi.hoisted(() => ({ getDatabaseMetrics: vi.fn(), configureDatabaseMonitor: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
vi.mock('../../api/cloud-resources.js', () => ({ getCloudConnections: vi.fn().mockResolvedValue([{ id: 4, name: '阿里云', provider: 'aliyun' }]) }))
const wrappers = []
const render = item => { const wrapper = mount(DatabaseMonitor, { props: { item }, global: { stubs: { Teleport: true, MetricTrendChart: { props: ['title', 'series', 'unit'], template: '<div data-testid="metric">{{ title }} {{ unit }}</div>' } } } }); wrappers.push(wrapper); return wrapper }
afterEach(() => { wrappers.splice(0).forEach(item => item.unmount()); vi.clearAllMocks() })
describe('RDS native metrics', () => {
  it('does not request metrics until a cloud account has been associated', async () => {
    const wrapper = render({ id: 1 })
    await flushPromises()
    expect(api.getDatabaseMetrics).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('关联阿里云账号')
  })
  it('renders provider metrics and clears stale values when loading fails', async () => {
    api.getDatabaseMetrics.mockResolvedValueOnce({ series: [{ key: 'cpu', label: 'CPU 使用率', unit: '%', values: [{ timestamp: 1, value: 25 }] }] }).mockRejectedValueOnce(new Error('RAM 权限不足'))
    const wrapper = render({ id: 1, cloud_connection_id: 4 })
    await flushPromises()
    expect(wrapper.get('[data-testid=metric]').text()).toBe('CPU 使用率 %')
    await wrapper.get('[aria-label="刷新监控"]').trigger('click'); await flushPromises()
    expect(wrapper.find('[data-testid=metric]').exists()).toBe(false)
    expect(wrapper.get('[role=alert]').text()).toContain('RAM 权限不足')
    expect(wrapper.text()).toContain('监控读取失败')
  })
})
