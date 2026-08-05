import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AlertingWorkspace from './AlertingWorkspace.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))

beforeEach(() => {
  apiMocks.get.mockReset()
  apiMocks.post.mockReset()
  apiMocks.put.mockReset()
  apiMocks.delete.mockReset()
})

describe('AlertingWorkspace', () => {
  it('guides the user to install alerting when it is not installed', async () => {
    apiMocks.get.mockResolvedValue({ state: 'not_installed', message: '尚未启用集群告警' })
    const wrapper = mount(AlertingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], monitoringReady: true, metricsNodeName: 'node-b' } })
    await flushPromises()

    expect(wrapper.text()).toContain('告警尚未启用')
    expect(wrapper.find('select').element.value).toBe('node-a')
    expect(wrapper.text()).toContain('飞书机器人地址')
  })

  it('prioritizes firing alerts and opens the centered settings modal', async () => {
    apiMocks.get.mockImplementation(path => {
      if (path === '/monitoring/alerts/status') return Promise.resolve({ state: 'ready', message: '告警规则正在评估', node_name: 'node-a', notification_configured: true, feishu_configured: true, notification_policy: { group_wait_seconds: 30, group_interval_minutes: 5, repeat_interval_minutes: 240 }, rules: [{ id: 'node-cpu-high', name: '节点 CPU 过高', severity: 'warning', enabled: true, threshold: 0, duration_minutes: 15 }] })
      if (path === '/monitoring/alerts/overview') return Promise.resolve({ firing: 1, silenced: 0, active: [{ status: { state: 'firing' }, labels: { alertname: 'NodeCPUHigh', node: 'node-a', severity: 'warning' }, annotations: { summary: '节点 CPU 使用率过高', current_value: '77.23%', threshold: '75%' }, startsAt: '2026-08-04T10:00:00Z' }], resolved: [{ status: { state: 'resolved' }, labels: { alertname: 'NodeMemoryHigh', node: 'node-a' }, annotations: { summary: '节点内存使用率已恢复' }, endsAt: '2026-08-04T10:10:00Z' }] })
      if (path === '/monitoring/alerts/silences') return Promise.resolve([])
      return Promise.resolve({})
    })
    const wrapper = mount(AlertingWorkspace, { props: { nodes: [{ name: 'node-a', ready: true }], monitoringReady: true } })
    await flushPromises()

    expect(wrapper.text()).toContain('正在告警')
    expect(wrapper.text()).toContain('节点 CPU 使用率过高')
    expect(wrapper.text()).toContain('77.23%')
    expect(wrapper.text()).toContain('75%')
    expect(wrapper.get('.alert-resolved').classes()).toContain('card')
    await wrapper.get('[title="告警设置"]').trigger('click')
    await flushPromises()
    const modal = document.body.querySelector('.alert-settings-modal')
    expect(modal).not.toBeNull()
    expect(modal.textContent).toContain('飞书通知')
    expect(modal.textContent).toContain('邮件通知')
    expect(modal.textContent).toContain('通知频率')
    expect(modal.textContent).toContain('已配置')
    expect(modal.textContent).toContain('阈值')
    const feishuTestButton = [...modal.querySelectorAll('button')].find(button => button.textContent.includes('测试飞书通知'))
    expect(feishuTestButton).toBeDefined()
    feishuTestButton.click()
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/monitoring/alerts/test-notification?channel=feishu', {})
    modal.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(apiMocks.put).toHaveBeenCalledWith('/monitoring/alerts/config', expect.objectContaining({ notification_policy: { group_wait_seconds: 30, group_interval_minutes: 5, repeat_interval_minutes: 240 } }))
    wrapper.unmount()
  })
})
