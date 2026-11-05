import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ComponentMigration from './ComponentMigration.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), start: vi.fn(), logs: vi.fn() }))
vi.mock('../../api/componentMigrations.js', () => ({ getComponentMigrations: mocks.list, startComponentMigration: mocks.start }))
vi.mock('../../api/clusters.js', () => ({ getClusterDeploymentLogs: mocks.logs }))
const nodes = [{ name: 'a', ready: true }, { name: 'b', ready: true }, { name: 'down', ready: false }, { name: 'cordoned', ready: true, evicted: true }]
const task = { id: 1, task_id: 10, status: 'copying', source_node_name: 'a', target_node_name: 'b', source_pvc_name: 'old', target_pvc_name: 'new', detail: '正在复制' }
let wrappers
function render(props = {}) {
  const wrapper = mount(ComponentMigration, { props: { clusterId: '2', component: 'loki', currentNode: 'a', nodes, ...props }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.useFakeTimers(); wrappers = []; Object.values(mocks).forEach(mock => mock.mockReset()); mocks.list.mockResolvedValue([]); mocks.logs.mockResolvedValue([]) })
afterEach(() => { wrappers.forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })

describe('ComponentMigration', () => {
  it('excludes the source and unavailable nodes and only starts after confirmation', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="open-component-migration"]').trigger('click')
    await flushPromises()
    const values = wrapper.findAll('select option').map(option => option.attributes('value'))
    expect(values).toEqual(['', 'b'])
    expect(mocks.start).not.toHaveBeenCalled()
    await wrapper.get('select').setValue('b')
    mocks.start.mockResolvedValue({ ...task, status: 'pending' })
    mocks.list.mockResolvedValue([task])
    await wrapper.get('[data-testid="start-component-migration"]').trigger('click')
    await flushPromises()
    expect(mocks.start).toHaveBeenCalledWith('2', 'loki', 'b', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('正在复制')
    expect(wrapper.emitted('busy').at(-1)).toEqual([true])
  })
  it('resumes progress after reload and continues polling when the dialog closes', async () => {
    mocks.list.mockResolvedValue([task])
    mocks.logs.mockResolvedValue([{ id: 1, message: '数据校验中' }])
    const wrapper = render({ component: 'victoria-metrics' })
    await flushPromises()
    expect(wrapper.text()).toContain('查看迁移进度')
    await wrapper.get('[data-testid="open-component-migration"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('数据校验中')
    expect(mocks.logs).toHaveBeenCalledWith('2', 10, 0, expect.anything())
    await wrapper.findAll('button').find(button => button.text() === '关闭（后台继续）').trigger('click')
    mocks.list.mockResolvedValue([{ ...task, status: 'succeeded', detail: '迁移成功' }])
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(wrapper.emitted('busy').at(-1)).toEqual([false])
    expect(wrapper.emitted('changed')).toHaveLength(1)
    const calls = mocks.list.mock.calls.length
    await vi.advanceTimersByTimeAsync(6000)
    expect(mocks.list.mock.calls).toHaveLength(calls)
  })
  it('ignores responses from the previous cluster', async () => {
    let resolve
    mocks.list.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = render()
    await wrapper.setProps({ clusterId: '3' })
    await flushPromises()
    resolve([task])
    await flushPromises()
    expect(wrapper.text()).not.toContain('查看迁移进度')
    expect(wrapper.emitted('busy').at(-1)).toEqual([false])
    expect(mocks.list).toHaveBeenLastCalledWith('3', 'loki', expect.anything())
  })
  it('shows failures through the shared error modal', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="open-component-migration"]').trigger('click')
    await flushPromises()
    await wrapper.get('select').setValue('b')
    mocks.start.mockRejectedValue(new Error('目标节点无法连接'))
    await wrapper.get('[data-testid="start-component-migration"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('目标节点无法连接')
    expect(wrapper.emitted('busy').at(-1)).toEqual([false])
  })
})
