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
              target_name: 'console.example.com',
              actor_name: 'admin',
              outcome: 'succeeded',
              summary: '发布平台版本',
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
    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.find('.section-tabs-header').exists()).toBe(true)
    expect(wrapper.find('.workspace-header').exists()).toBe(false)
    expect(wrapper.get('[data-testid="audit-open-filters"]').classes()).not.toContain('btn-sm')
    expect(wrapper.get('[data-testid="audit-apply-filters"]').classes()).not.toContain('btn-sm')
    expect(wrapper.get('.audit-card').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(wrapper.get('.audit-card').classes()).toContain('surface-card--padding-none')
    expect(wrapper.find('.audit-table-toolbar').exists()).toBe(true)
    expect(wrapper.find('.audit-filter-panel').exists()).toBe(false)
    expect(wrapper.get('[data-testid="audit-page-logs"]').text()).toBe('审计记录')
    expect(wrapper.text()).toContain('console.example.com')
    expect(wrapper.text()).toContain('发布平台版本')
    expect(wrapper.findAll('.audit-card table')).toHaveLength(1)
    expect(wrapper.findAll('.audit-row').every(row => !row.classes().includes('surface-card'))).toBe(true)

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
    expect(wrapper.text()).toContain('执行 创建：站点 #1')

    apiMocks.get.mockRejectedValueOnce(new Error('审计服务暂时不可用'))
    await wrapper.get('.audit-search').setValue('missing')
    await wrapper.get('[data-testid="audit-apply-filters"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('执行 创建：站点 #1')
    expect(wrapper.text()).toContain('审计服务暂时不可用')
  })

  it('edits advanced filters in a modal and exposes active chips', async () => {
    const wrapper = mount(AuditLogs, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    await wrapper.get('[data-testid="audit-open-filters"]').trigger('click')
    const modal = wrapper.get('.audit-filter-modal')
    await modal.find('select[aria-label="操作者类型"]').setValue('agent')
    await modal.find('input[type="date"]').setValue('2026-09-01')
    await modal.findAll('input[type="date"]')[1].setValue('2026-09-08')
    await modal.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('操作者：Agent')
    expect(apiMocks.get.mock.calls.some(([path]) => path.includes('actor_type=agent') && path.includes('created_from=2026-09-01') && path.includes('created_to=2026-09-08'))).toBe(true)
  })
})
