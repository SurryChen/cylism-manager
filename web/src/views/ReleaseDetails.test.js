import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ReleaseDetails from './ReleaseDetails.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation((path) => {
      if (path === '/applications/1') return Promise.resolve({ application: { id: 1, name: 'order-api' }, releases: [] })
      return Promise.resolve({ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded', operations: [{ id: 4, step: 'wait_ready', status: 'success', detail: '' }], runtime: { tracking: 'exact', diagnostic: 'Pod order-api-abc 的容器 api: CrashLoopBackOff；曾正常退出 (退出码 0)，但已重启 4 次', pods: [{ name: 'order-api-abc', node_name: 'worker-a', phase: 'Running', ready: false, restarts: 4, containers: [{ name: 'api', state: 'waiting', reason: 'CrashLoopBackOff', last_reason: 'Completed', last_exit_code: 0 }] }] } })
    }),
    post: vi.fn(),
  },
}))

describe('ReleaseDetails view', () => {
  it('shows release operations on a dedicated drill-down page', async () => {
    const wrapper = mount(ReleaseDetails, {
      props: { applicationID: '1', releaseID: '3' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.back-link').text()).toContain('返回 order-api')
    expect(wrapper.find('.page-title').text()).toBe('Release #2')
    expect(wrapper.text()).toContain('wait_ready')
    expect(wrapper.text()).toContain('当前运行状态')
    expect(wrapper.text()).toContain('CrashLoopBackOff')
    expect(wrapper.text()).toContain('worker-a')
  })
})
