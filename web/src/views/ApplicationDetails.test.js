import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ApplicationDetails from './ApplicationDetails.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockResolvedValue({
      application: { id: 1, name: 'order-api', project: { name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } },
      releases: [{ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded' }],
    }),
  },
}))

describe('ApplicationDetails view', () => {
  it('shows release history on a dedicated application drill-down page', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.back-link').text()).toContain('返回应用列表')
    expect(wrapper.find('.page-title').text()).toBe('order-api')
    expect(wrapper.text()).toContain('Release #2')
    expect(wrapper.text()).toContain('registry.example.com/order-api:2.0.0')
  })
})
