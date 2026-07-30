import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ApplicationDetails from './ApplicationDetails.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/applications/1') return Promise.resolve({ application: { id: 1, project_id: 2, environment_id: 3, name: 'order-api', project: { name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } }, releases: [{ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded' }] })
      if (path === '/applications/1/deployment-templates') return Promise.resolve([{ id: 4, name: '标准生产配置', enabled: true, is_default: true, revision: 2, spec: { image: 'registry.example.com/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      if (path === '/applications/1/endpoint') return Promise.resolve({ domain: 'api.example.com', path: '/', service_port: 80, tls_enabled: true })
      return Promise.resolve([])
    }),
    post: vi.fn(), put: vi.fn(), delete: vi.fn(),
  },
}))

describe('ApplicationDetails view', () => {
  it('shows release history on a dedicated application drill-down page', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.back-link').text()).toContain('返回工作台')
    expect(wrapper.find('.page-title').text()).toBe('order-api')
    expect(wrapper.text()).toContain('Release #2')
    expect(wrapper.text()).toContain('registry.example.com/order-api:2.0.0')
    expect(wrapper.text()).toContain('标准生产配置')
    expect(wrapper.text()).toContain('api.example.com')
  })
})
