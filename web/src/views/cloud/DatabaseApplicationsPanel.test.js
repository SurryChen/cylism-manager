import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseApplicationsPanel from './DatabaseApplicationsPanel.vue'
const getDatabaseApplications = vi.hoisted(() => vi.fn())
vi.mock('../../api/databases.js', () => ({ getDatabaseApplications }))
const wrappers = []
const render = () => { const wrapper = mount(DatabaseApplicationsPanel, { props: { instanceId: 1 }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper); return wrapper }
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.resetAllMocks() })

describe('database application connections', () => {
  it('shows detected file connections without requiring a global business credential', async () => {
    getDatabaseApplications.mockResolvedValue({ items: [{ application_id: 13, application_name: 'account', template_id: 2, template_name: 'production', database_name: 'cylism_account', username: 'account', sources: ['文件 · /app/config.yaml:MYSQL_DSN'] }, { application_id: 16, application_name: 'bill', template_id: 3, template_name: 'default', database_name: 'cylism_bill', username: 'bill', binding_id: 1, sources: ['平台绑定'] }], warnings: [] })
    const wrapper = render(); await flushPromises()
    expect(getDatabaseApplications).toHaveBeenCalledWith(1)
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.text()).toContain('cylism_account')
    expect(wrapper.text()).toContain('配置识别')
    expect(wrapper.text()).toContain('已绑定')
    expect(wrapper.get('a').attributes('href')).toBe('#/applications/13?tab=config')
  })

  it('surfaces incomplete scans and read failures instead of claiming no connections', async () => {
    getDatabaseApplications.mockResolvedValueOnce({ items: [], warnings: ['account：Secret 无法读取'] })
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('查看原因')).trigger('click')
    expect(wrapper.get('[role=alert]').text()).toContain('Secret 无法读取')
    getDatabaseApplications.mockRejectedValueOnce(new Error('读取接口不可用'))
    await wrapper.get('[aria-label="刷新应用连接"]').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('应用连接读取失败')
    expect(wrapper.text()).not.toContain('未识别到应用连接')
  })

  it('does not show results from the previous instance after switching', async () => {
    let finish
    getDatabaseApplications.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    getDatabaseApplications.mockResolvedValueOnce({ items: [], warnings: [] })
    const wrapper = render()
    await wrapper.setProps({ instanceId: 2 }); await flushPromises()
    finish({ items: [{ application_name: 'wrong instance' }], warnings: [] }); await flushPromises()
    expect(wrapper.text()).not.toContain('wrong instance')
    expect(wrapper.text()).toContain('未识别到应用连接')
  })
})
