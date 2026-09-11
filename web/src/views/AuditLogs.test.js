import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AuditLogs from './AuditLogs.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../api/index.js', () => ({ api: apiMocks }))

describe('AuditLogs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockImplementation((path) => {
      if (path.startsWith('/audit-logs')) {
        return Promise.resolve({
          data: [
            {
              id: 1,
              action: 'deploy',
              resource_type: 'platform',
              resource_id: 9,
              user_id: 2,
              detail: JSON.stringify({ domain: 'console.example.com', request: { domain: 'console.example.com' }, message: 'ok' }),
              created_at: '2026-09-08T07:16:22Z',
            },
          ],
          total: 1,
        })
      }
      return Promise.resolve([])
    })
  })

  it('filters logs and opens detail drawer', async () => {
    const wrapper = mount(AuditLogs, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('审计日志')
    expect(wrapper.text()).toContain('console.example.com')

    await wrapper.get('.audit-search').setValue('console')
    await wrapper.get('[data-testid="audit-apply-filters"]').trigger('click')
    expect(apiMocks.get.mock.calls.some(([path]) => path.includes('keyword=console'))).toBe(true)
    expect(apiMocks.get.mock.calls.find(([path]) => path.includes('keyword=console'))[1]).toEqual(expect.objectContaining({ signal: expect.any(AbortSignal) }))

    await wrapper.get('.audit-row').trigger('click')
    expect(wrapper.find('.audit-detail-modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('结构化内容')
    expect(wrapper.text()).toContain('console.example.com')
    expect(wrapper.text()).toContain('原始详情')
  })

  it('retains the last successful page when a later filter read fails', async () => {
    apiMocks.get.mockResolvedValueOnce({ data: [{ id: 7, action: 'create', resource_type: 'site', resource_id: 1, detail: 'initial' }], total: 1 })
    const wrapper = mount(AuditLogs, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('initial')

    apiMocks.get.mockRejectedValueOnce(new Error('审计服务暂时不可用'))
    await wrapper.get('.audit-search').setValue('missing')
    await wrapper.get('[data-testid="audit-apply-filters"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('initial')
    expect(wrapper.text()).toContain('审计服务暂时不可用')
  })
})
