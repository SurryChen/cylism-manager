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
    expect(apiMocks.get).toHaveBeenCalledWith(expect.stringContaining('keyword=console'))

    await wrapper.get('.audit-row').trigger('click')
    expect(wrapper.find('.audit-detail-modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('结构化内容')
    expect(wrapper.text()).toContain('console.example.com')
    expect(wrapper.text()).toContain('原始详情')
  })
})
