import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseResourceDialog from './DatabaseResourceDialog.vue'
import SelectMenu from '../../components/SelectMenu.vue'
const api = vi.hoisted(() => ({ updateDatabaseResources: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
const wrappers = []
afterEach(() => { wrappers.splice(0).forEach(item => item.unmount()); vi.clearAllMocks() })
const render = () => { const wrapper = mount(DatabaseResourceDialog, { props: { instance: { id: 2, name: 'replica', cpu_limit: '1', memory_limit: '1Gi' } }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper); return wrapper }
describe('standalone MySQL resources', () => {
  it('submits the chosen budget and opens the queued task', async () => {
    api.updateDatabaseResources.mockResolvedValue({ id: 8 })
    const wrapper = render()
    expect(wrapper.text()).toContain('重启 MySQL')
    const select = wrapper.findComponent(SelectMenu)
    select.vm.$emit('update:modelValue', '2/2Gi'); select.vm.$emit('change', '2/2Gi')
    await flushPromises()
    expect(wrapper.get('[aria-label="内存上限"]').element.value).toBe('2Gi')
    await wrapper.get('.btn-primary').trigger('click'); await flushPromises()
    expect(api.updateDatabaseResources).toHaveBeenCalledWith(2, { cpu_limit: '2', memory_limit: '2Gi' })
    expect(wrapper.emitted('created')).toEqual([[8]])
  })
  it('preserves the form and reports a locked resource', async () => {
    api.updateDatabaseResources.mockRejectedValue(new Error('资源仍被数据库任务占用'))
    const wrapper = render()
    await wrapper.get('[aria-label="CPU 上限"]').setValue('500m')
    await wrapper.get('.btn-primary').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('数据库任务占用')
    expect(wrapper.get('[aria-label="CPU 上限"]').element.value).toBe('500m')
    expect(wrapper.emitted('created')).toBeUndefined()
  })
})
