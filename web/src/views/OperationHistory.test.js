import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OperationHistory from './OperationHistory.vue'

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
    expect(wrapper.get('[data-testid="operation-page-history"]').text()).toBe('执行记录')
    expect(wrapper.text()).toContain('等待工作负载就绪')
    expect(apiMocks.get).toHaveBeenCalledWith('/operations?limit=20&offset=0', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })
})
