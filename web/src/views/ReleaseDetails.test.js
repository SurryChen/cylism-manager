import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ReleaseDetails from './ReleaseDetails.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation((path) => {
      if (path === '/applications/1') return Promise.resolve({ application: { id: 1, name: 'order-api' }, releases: [] })
      return Promise.resolve({ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded', operations: [{ id: 4, step: 'wait_ready', status: 'success', detail: '' }] })
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
  })
})
