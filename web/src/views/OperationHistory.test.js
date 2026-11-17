import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OperationHistory from './OperationHistory.vue'
import PaginationControls from '../components/PaginationControls.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../api/index.js', () => ({ api: apiMocks }))

describe('OperationHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockResolvedValue({ operations: [{ id: 1, resource_type: 'application', resource_id: 3, step: '等待工作负载就绪', status: 'failed', detail: 'readiness probe failed', created_at: '2026-09-15T01:00:00Z' }], total: 1 })
  })

  it('loads workflow steps from the independent operations endpoint', async () => {
    const wrapper = mount(OperationHistory)
    await flushPromises()
    expect(wrapper.text()).toContain('操作历史')
    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.find('.section-tabs-header').exists()).toBe(true)
    expect(wrapper.find('.workspace-header').exists()).toBe(false)
    expect(wrapper.get('.operation-card').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(wrapper.get('.operation-card').classes()).toContain('surface-card--padding-none')
    expect(wrapper.find('.operation-table-toolbar').exists()).toBe(true)
    expect(wrapper.get('[data-testid="operation-page-history"]').text()).toBe('执行记录')
    expect(wrapper.text()).toContain('等待工作负载就绪')
    expect(wrapper.findAll('.operation-card table')).toHaveLength(1)
    expect(wrapper.findAll('tbody tr').every(row => !row.classes().includes('surface-card'))).toBe(true)
    expect(apiMocks.get).toHaveBeenCalledWith('/operations?limit=10&offset=0', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('keeps the current page after failed navigation and supports size changes and filter resets', async () => {
    apiMocks.get.mockImplementation(async path => {
      const offset = Number(new URLSearchParams(path.split('?')[1]).get('offset'))
      return { operations: [{ id: offset + 1, step: '步骤 ' + (offset + 1) }], total: 35 }
    })
    const wrapper = mount(OperationHistory)
    await flushPromises()
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    expect(apiMocks.get.mock.lastCall[0]).toBe('/operations?limit=10&offset=10')
    apiMocks.get.mockRejectedValueOnce(new Error('暂时不可用'))
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    expect(wrapper.getComponent(PaginationControls).props('page')).toBe(2)
    expect(wrapper.text()).toContain('步骤 11')
    expect(wrapper.get('[role="alert"]').text()).toContain('暂时不可用')
    await wrapper.get('[data-testid="dismiss-error-notice"]').trigger('click')
    await wrapper.get('select[aria-label="每页条数"]').setValue('20')
    await flushPromises()
    expect(apiMocks.get.mock.lastCall[0]).toBe('/operations?limit=20&offset=0')
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    await wrapper.get('.operation-search').setValue('test')
    await wrapper.get('.operation-search').trigger('keydown.enter')
    await flushPromises()
    expect(apiMocks.get.mock.lastCall[0]).toBe('/operations?limit=20&offset=0&keyword=test')
    expect(wrapper.getComponent(PaginationControls).props('page')).toBe(1)
    wrapper.unmount()
  })

  it('retains visible rows while the next page loads to avoid collapsing the table', async () => {
    apiMocks.get.mockResolvedValueOnce({ operations: [{ id: 1, step: '当前步骤' }], total: 35 })
    const wrapper = mount(OperationHistory)
    await flushPromises()
    let resolveNextPage
    apiMocks.get.mockImplementationOnce(() => new Promise(resolve => { resolveNextPage = resolve }))
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    expect(wrapper.text()).toContain('当前步骤')
    expect(wrapper.get('.table-wrap').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('[aria-label="下一页"]').element.disabled).toBe(true)
    resolveNextPage({ operations: [{ id: 11, step: '下一页步骤' }], total: 35 })
    await flushPromises()
    expect(wrapper.get('.table-wrap').attributes('aria-busy')).toBe('false')
    expect(wrapper.text()).toContain('下一页步骤')
    expect(wrapper.getComponent(PaginationControls).props('page')).toBe(2)
    wrapper.unmount()
  })
})
