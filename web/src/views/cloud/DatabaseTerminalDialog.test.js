import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseTerminalDialog from './DatabaseTerminalDialog.vue'

const api = vi.hoisted(() => ({ createDatabaseSession: vi.fn(), deleteDatabaseSession: vi.fn(), getDatabaseExecutionNodes: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
const wrappers = []
const instance = { id: 2, name: 'Replica', mode: 'managed', cluster_id: 1, node_name: 'worker' }
const render = () => {
  const wrapper = mount(DatabaseTerminalDialog, { props: { instance, clusters: [{ id: 1, name: '自用' }] }, global: { stubs: { Teleport: true, PodTerminal: { name: 'PodTerminal', props: ['databaseSession'], emits: ['close', 'reconnect', 'configure'], template: '<div />' } } } })
  wrappers.push(wrapper)
  return wrapper
}
const open = async wrapper => {
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === '打开终端').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  api.getDatabaseExecutionNodes.mockResolvedValue([{ name: 'worker', ready: true }])
  api.createDatabaseSession.mockResolvedValue({ id: 'first', instance_id: 2, username: 'root', node_name: 'worker' })
  api.deleteDatabaseSession.mockResolvedValue({})
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('database terminal sessions', () => {
  it('deletes the temporary client when closing an open terminal', async () => {
    const wrapper = render()
    await open(wrapper)
    expect(api.createDatabaseSession).toHaveBeenCalledWith(2, { account: 'admin', cluster_id: 1, node_name: 'worker' })
    wrapper.getComponent({ name: 'PodTerminal' }).vm.$emit('close')
    await flushPromises()
    expect(api.deleteDatabaseSession).toHaveBeenCalledWith(2, 'first')
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.findComponent({ name: 'PodTerminal' }).exists()).toBe(false)
  })

  it('uses a new session when reconnecting and cleans up when changing nodes', async () => {
    const wrapper = render()
    await open(wrapper)
    api.createDatabaseSession.mockResolvedValueOnce({ id: 'second', instance_id: 2, username: 'root', node_name: 'worker' })
    wrapper.getComponent({ name: 'PodTerminal' }).vm.$emit('reconnect')
    await flushPromises()
    expect(api.deleteDatabaseSession).toHaveBeenCalledWith(2, 'first')
    expect(wrapper.getComponent({ name: 'PodTerminal' }).props('databaseSession').id).toBe('second')
    wrapper.getComponent({ name: 'PodTerminal' }).vm.$emit('configure')
    await flushPromises()
    expect(api.deleteDatabaseSession).toHaveBeenCalledWith(2, 'second')
    expect(wrapper.findComponent({ name: 'PodTerminal' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('登录数据库终端')
  })
})
