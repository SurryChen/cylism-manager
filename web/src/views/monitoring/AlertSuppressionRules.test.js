import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AlertSuppressionRules from './AlertSuppressionRules.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: apiMocks }))
const wrappers = []
function render() { const wrapper = mount(AlertSuppressionRules, { props: { clusterId: '2' } }); wrappers.push(wrapper); return wrapper }
const rule = { id: 9, name: 'cloudcone 资源告警', node_name: 'cloudcone', alert_names: ['NodeCPUHigh', 'NodeMemoryHigh'], enabled: true, ends_at: null }
beforeEach(() => { Object.values(apiMocks).forEach(mock => mock.mockReset()); apiMocks.get.mockResolvedValue({ items: [rule], total: 1 }) })
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('AlertSuppressionRules', () => {
  it('creates a permanent node and multi-type rule', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('.suppression-create').trigger('click')
    await wrapper.get('[aria-label="屏蔽规则名称"]').setValue('cloudcone CPU 和内存')
    await wrapper.get('[aria-label="屏蔽节点"]').setValue('cloudcone')
    await wrapper.get('input[value="NodeCPUHigh"]').setValue(true)
    await wrapper.get('input[value="NodeMemoryHigh"]').setValue(true)
    await wrapper.get('select[aria-label="屏蔽有效期"]').setValue('0')
    await wrapper.get('#suppression-rule-form').trigger('submit')
    await flushPromises()
    expect(apiMocks.post).toHaveBeenCalledWith('/clusters/2/monitoring/alerts/suppression-rules', expect.objectContaining({ name: 'cloudcone CPU 和内存', node_name: 'cloudcone', alert_names: ['NodeCPUHigh', 'NodeMemoryHigh'], duration_minutes: 0, enabled: true }), { clusterID: '2' })
    expect(wrapper.find('#suppression-rule-form').exists()).toBe(false)
  })

  it('preserves expiration during an edit and supports explicit empty types', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('.suppression-actions .btn')[0].trigger('click')
    expect(wrapper.get('select[aria-label="屏蔽有效期"]').element.value).toBe('keep')
    await wrapper.get('.suppression-types > .checkbox-label input').setValue(true)
    await wrapper.get('#suppression-rule-form').trigger('submit')
    await flushPromises()
    const [path, body] = apiMocks.put.mock.lastCall
    expect(path).toBe('/clusters/2/monitoring/alerts/suppression-rules/9')
    expect(body.alert_names).toEqual([])
    expect(body).not.toHaveProperty('duration_minutes')
  })

  it('requires a specific scope and keeps the editor open on failure', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('.suppression-create').trigger('click')
    await wrapper.get('[aria-label="屏蔽规则名称"]').setValue('全节点')
    await wrapper.get('.suppression-types > .checkbox-label input').setValue(true)
    expect(wrapper.get('button[form="suppression-rule-form"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[aria-label="屏蔽节点"]').setValue('cloudcone')
    apiMocks.post.mockRejectedValueOnce(new Error('保存失败'))
    await wrapper.get('#suppression-rule-form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('#suppression-rule-form').exists()).toBe(true)
    expect(wrapper.text()).toContain('保存失败')
  })

  it('toggles and confirms deletion without discarding the list on failure', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('.suppression-actions .btn')[1].trigger('click')
    await flushPromises()
    expect(apiMocks.patch).toHaveBeenCalledWith('/clusters/2/monitoring/alerts/suppression-rules/9/enabled', { enabled: false }, { clusterID: '2' })
    await wrapper.get('[aria-label="删除规则：cloudcone 资源告警"]').trigger('click')
    expect(apiMocks.delete).not.toHaveBeenCalled()
    apiMocks.delete.mockRejectedValueOnce(new Error('删除失败'))
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('删除失败')
    expect(wrapper.find('.data-table').exists()).toBe(true)
  })
})
