import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseTaskDialog from './DatabaseTaskDialog.vue'

const api = vi.hoisted(() => ({ controlDatabaseOperation: vi.fn(), getDatabaseOperation: vi.fn(), getDatabaseOperations: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
const wrappers = []
const button = (wrapper, text) => wrapper.findAll('button').find(item => item.text() === text)
beforeEach(() => {
  vi.clearAllMocks()
  api.getDatabaseOperations.mockResolvedValue({ items: [{ id: 7, kind: 'migrate', status: 'running', phase_name: '导入数据库' }], total: 1 })
  api.getDatabaseOperation.mockResolvedValue({ task: { id: 7, kind: 'migrate', status: 'running', phase: 2, progress: '执行中', phase_name: '导入数据库' }, phases: ['检查', '导出', '导入', '核验'], logs: [] })
  api.controlDatabaseOperation.mockResolvedValue({})
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('durable database task controls', () => {
  it.each([{ schemas: 'app,bill' }, { schemas: ['app', 'bill'] }])('renders discovered schemas $schemas and each replication thread accurately', async ({ schemas }) => {
    api.getDatabaseOperation.mockResolvedValue({ task: { id: 7, kind: 'replication_discover', status: 'succeeded', phase: 1, result: JSON.stringify({ schemas, io_running: true, sql_running: false, lag_seconds: null }) }, phases: [], logs: [] })
    const wrapper = mount(DatabaseTaskDialog, { props: { open: true, taskID: 7 }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper)
    await flushPromises()
    expect(wrapper.text()).toContain('业务数据库：app、bill')
    expect(wrapper.text()).toContain('复制接收线程：运行中；执行线程：未运行；延迟：未知')
  })

  it('loads the selected task and requests checkpoint pause', async () => {
    const wrapper = mount(DatabaseTaskDialog, { props: { open: true, taskID: 7 }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper)
    await flushPromises()
    expect(api.getDatabaseOperation).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('核验')
    api.getDatabaseOperation.mockResolvedValue({ task: { id: 7, kind: 'migrate', status: 'running', pause_requested: true, phase: 2 }, phases: [], logs: [] })
    await button(wrapper, '暂停').trigger('click'); await flushPromises()
    expect(api.controlDatabaseOperation).toHaveBeenCalledWith(7, 'pause')
    expect(button(wrapper, '等待检查点暂停').attributes('disabled')).toBeDefined()
  })

  it('requires confirmation before cancellation and disables repeat actions while submitting', async () => {
    const wrapper = mount(DatabaseTaskDialog, { props: { open: true, taskID: 7 }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper)
    await flushPromises()
    await button(wrapper, '终止').trigger('click')
    expect(api.controlDatabaseOperation).not.toHaveBeenCalled()
    let finish
    api.controlDatabaseOperation.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    expect(wrapper.get('[data-testid="confirm-dialog-confirm"]').attributes('disabled')).toBeDefined()
    expect(api.controlDatabaseOperation).toHaveBeenCalledWith(7, 'cancel')
    finish({}); await flushPromises()
  })
})
