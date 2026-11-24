import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseInstanceMonitor from './DatabaseInstanceMonitor.vue'
const api = vi.hoisted(() => ({ getDatabaseMetrics: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
const wrappers = []
const render = () => { const wrapper = mount(DatabaseInstanceMonitor, { props: { instanceId: 2 }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper); return wrapper }
afterEach(() => { wrappers.splice(0).forEach(item => item.unmount()); vi.clearAllMocks(); vi.useRealTimers() })
const sample = { checked_at: '2026-11-08T01:00:00Z', metrics_available: true, cpu_millicores: 80, memory_bytes: 600 * 1048576, cpu_limit_millicores: 2000, memory_limit_bytes: 2147483648, ready: true, phase: 'Running', node_name: 'ubuntudev', restarts: 2, last_termination: 'OOMKilled' }
describe('standalone MySQL usage', () => {
  it('shows usage against the actual running budget and refreshes without tasks', async () => {
    api.getDatabaseMetrics.mockResolvedValue(sample)
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('600')
    expect(wrapper.text()).toContain('4.0%')
    expect(wrapper.text()).toContain('29.3%')
    expect(wrapper.text()).toContain('OOMKilled')
    expect(wrapper.text()).toContain('上限 2.00 核')
    await wrapper.get('[aria-label="刷新资源使用量"]').trigger('click'); await flushPromises()
    expect(api.getDatabaseMetrics).toHaveBeenCalledTimes(2)
  })
  it('shows missing metrics instead of zero usage and clears stale samples on failure', async () => {
    api.getDatabaseMetrics.mockResolvedValueOnce({ ...sample, metrics_available: false, cpu_millicores: null, memory_bytes: null, metrics_message: 'Metrics Server 尚未就绪' }).mockRejectedValueOnce(new Error('节点连接失败'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('暂时没有资源指标')
    expect(wrapper.text()).toContain('Metrics Server 尚未就绪')
    expect(wrapper.find('.usage-stats').exists()).toBe(false)
    await wrapper.get('[aria-label="刷新资源使用量"]').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('节点连接失败')
    expect(wrapper.text()).not.toContain('OOMKilled')
  })
})
