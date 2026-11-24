import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AlertHistory from './AlertHistory.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: apiMocks }))
const wrappers = []
function render(clusterId = '2') { const wrapper = mount(AlertHistory, { props: { clusterId } }); wrappers.push(wrapper); return wrapper }
const occurrence = { id: 1, node_name: 'cloudcone', alert_name: 'NodeCPUHigh', starts_at: '2026-11-21T12:00:00Z', severity: 'warning', status: 'resolved', summary: 'CPU 使用率过高', ends_at: '2026-11-21T12:10:00Z' }
beforeEach(() => { apiMocks.get.mockReset() })
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('AlertHistory', () => {
  it('filters server history and paginates with the applied filters', async () => {
    apiMocks.get.mockResolvedValue({ items: [occurrence], total: 21 })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('CPU 使用率过高')
    expect(wrapper.text()).toContain('已恢复')
    await wrapper.get('[aria-label="历史告警节点"]').setValue('cloudcone')
    await wrapper.get('select[aria-label="历史告警时间范围"]').setValue('30')
    await wrapper.get('select[aria-label="历史告警类型"]').setValue('NodeCPUHigh')
    await wrapper.get('.history-toolbar').trigger('submit')
    await flushPromises()
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    const [path] = apiMocks.get.mock.lastCall
    const url = new URL(path, 'http://test')
    expect(url.pathname).toBe('/clusters/2/monitoring/alerts/history')
    expect(Object.fromEntries(url.searchParams)).toMatchObject({ node: 'cloudcone', days: '30', alert_name: 'NodeCPUHigh', offset: '10', limit: '10' })
  })

  it('renders frequency counts and keeps the previous snapshot on failure', async () => {
    apiMocks.get.mockResolvedValueOnce({ items: [occurrence], total: 1 })
    const wrapper = render()
    await flushPromises()
    apiMocks.get.mockResolvedValueOnce({ items: [{ node_name: 'cloudcone', alert_name: 'NodeCPUHigh', count: 27, last_seen: occurrence.starts_at }], total: 1 })
    await wrapper.findAll('.section-tab')[1].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('th').map(item => item.text())).toEqual(['节点', '告警类型', '触发次数', '最近触发', '操作'])
    expect(wrapper.text()).toContain('27')
    expect(apiMocks.get.mock.lastCall[0]).toContain('view=frequency')
    apiMocks.get.mockRejectedValueOnce(new Error('历史服务暂时不可用'))
    await wrapper.get('[aria-label="刷新告警记录"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('历史服务暂时不可用')
    expect(wrapper.text()).toContain('27')
  })

  it('aborts old cluster requests and clears old rows on a cluster switch', async () => {
    let resolveOld
    apiMocks.get.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    const signal = apiMocks.get.mock.calls[0][1].signal
    apiMocks.get.mockResolvedValueOnce({ items: [], total: 0 })
    await wrapper.setProps({ clusterId: '3' })
    await flushPromises()
    expect(signal.aborted).toBe(true)
    resolveOld({ items: [occurrence], total: 1 })
    await flushPromises()
    expect(wrapper.text()).not.toContain('CPU 使用率过高')
    expect(wrapper.text()).toContain('暂无告警记录')
    expect(apiMocks.get.mock.lastCall[0]).toContain('/clusters/3/')
  })

  it('prefills a suppression rule from a historical occurrence', async () => {
    apiMocks.get.mockResolvedValue({ items: [occurrence], total: 1 })
    const wrapper = render()
    await flushPromises()
    apiMocks.get.mockResolvedValue({ items: [], total: 0 })
    await wrapper.get('.data-table .btn').trigger('click')
    await flushPromises()
    expect(wrapper.get('[aria-label="屏蔽节点"]').element.value).toBe('cloudcone')
    expect(wrapper.get('input[value="NodeCPUHigh"]').element.checked).toBe(true)
    expect(wrapper.get('[aria-label="屏蔽规则名称"]').element.value).toContain('cloudcone')
    expect(apiMocks.get.mock.lastCall[0]).toContain('/clusters/2/monitoring/alerts/suppression-rules?')
  })
})
