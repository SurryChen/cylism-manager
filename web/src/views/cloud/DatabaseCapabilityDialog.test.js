import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseCapabilityDialog from './DatabaseCapabilityDialog.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const api = vi.hoisted(() => ({ createDatabase: vi.fn(), createDatabaseOperation: vi.fn(), getDatabaseBindings: vi.fn(), getDatabaseExecutionNodes: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
const wrappers = []
const button = (wrapper, text) => wrapper.findAll('button').find(item => item.text() === text)
const menu = (wrapper, label) => wrapper.findAllComponents(SelectMenu).find(item => item.props('ariaLabel') === label)
function render(kind, extra = {}) {
  const wrapper = mount(DatabaseCapabilityDialog, { props: { kind, clusters: [{ id: 1, name: '自用' }], instances: [{ id: 1, name: 'RDS', mode: 'external' }, { id: 2, name: '独立 MySQL', mode: 'managed' }], ...extra }, global: { stubs: { Teleport: true } } })
  wrappers.push(wrapper); return wrapper
}
async function chooseNode(wrapper) {
  menu(wrapper, '执行集群').vm.$emit('update:modelValue', 1); await flushPromises()
  menu(wrapper, '执行节点').vm.$emit('update:modelValue', 'ubuntudev'); await flushPromises()
}
beforeEach(() => {
  vi.clearAllMocks()
  api.getDatabaseExecutionNodes.mockResolvedValue([{ name: 'ubuntudev', database_allowed: true }])
  api.getDatabaseBindings.mockResolvedValue([])
  api.createDatabase.mockResolvedValue({ id: 2 })
  api.createDatabaseOperation.mockResolvedValue({ id: 7 })
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('independent database capabilities', () => {
  it('deploys an empty MySQL with an explicit node and budget without inventing a replica', async () => {
    const wrapper = render('deploy')
    await wrapper.get('input[maxlength="128"]').setValue('独立数据库')
    await chooseNode(wrapper)
    await button(wrapper, '创建后台任务').trigger('click'); await flushPromises()
    expect(api.createDatabase).toHaveBeenCalledWith(expect.objectContaining({ mode: 'managed', name: '独立数据库', cluster_id: 1, node_name: 'ubuntudev', cpu_limit: '1', memory_limit: '1Gi' }))
    expect(api.createDatabase.mock.calls[0][0]).not.toHaveProperty('source')
    expect(api.createDatabase.mock.calls[0][0]).not.toHaveProperty('password')
    expect(api.createDatabaseOperation).not.toHaveBeenCalled()
    expect(wrapper.emitted('created')).toEqual([[null]])
  })

  it('does not preselect a replication target and clears credentials on close', async () => {
    const wrapper = render('replicate', { source: 'instance:1' })
    expect(menu(wrapper, '目标数据库').props('modelValue')).toBe('')
    expect(menu(wrapper, '目标数据库').props('options')).toEqual([{ value: '', label: '选择目标数据库' }, { value: 'instance:2', label: '独立 MySQL' }])
    await wrapper.get('input[type="password"]').setValue('discarded-password')
    await button(wrapper, '取消').trigger('click')
    expect(wrapper.get('input[type="password"]').element.value).toBe('')
    expect(api.createDatabaseOperation).not.toHaveBeenCalled()
  })

  it('checks a selected execution node without a dependency on all node DNS probes', async () => {
    const wrapper = render('inspect', { source: 'instance:1' })
    await chooseNode(wrapper)
    await button(wrapper, '创建后台任务').trigger('click'); await flushPromises()
    expect(api.createDatabaseOperation).toHaveBeenCalledWith({ kind: 'inspect', source: 'instance:1', target: '', cluster_id: 1, node_name: 'ubuntudev', image: '' })
    expect(wrapper.emitted('created')).toEqual([[7]])
  })

  it('only offers real replication targets for detach', async () => {
    const wrapper = render('detach')
    expect(menu(wrapper, '目标数据库').props('options')).toHaveLength(1)
    await wrapper.setProps({ relations: [{ source: 'instance:1', target: 'instance:2', status: 'running' }] })
    expect(menu(wrapper, '目标数据库').props('options')).toHaveLength(2)
  })
})
