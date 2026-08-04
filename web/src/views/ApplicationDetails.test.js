import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ApplicationDetails from './ApplicationDetails.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/applications/1') return Promise.resolve({ application: { id: 1, project_id: 2, environment_id: 3, name: 'order-api', workload_kind: 'deployment', project: { name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } }, releases: [{ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded' }] })
      if (path === '/applications/1/deployment-templates') return Promise.resolve([{ id: 4, name: '标准生产配置', enabled: true, is_default: true, revision: 2, spec: { image: 'registry.example.com/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      if (path === '/applications/1/endpoints') return Promise.resolve([{ id: 7, domain_id: 4, domain: 'api.example.com', path: '/', service_port: 80, tls_enabled: true }, { id: 8, domain_id: 5, domain: 'admin.example.com', path: '/console', service_port: 80, tls_enabled: false }])
      if (path === '/domains?environment_id=3') return Promise.resolve([{ id: 4, hostname: 'api.example.com', enabled: true, certificate: { status: 'Ready' } }, { id: 5, hostname: 'admin.example.com', enabled: true, certificate: { status: 'Ready' } }])
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
    expect(wrapper.text()).toContain('admin.example.com')
    expect(wrapper.find('[aria-label="工作负载类型"]').element.value).toBe('deployment')
  })

  it('serializes line-based startup command and arguments into the template spec', async () => {
    const { api } = await import('../api/index.js')
    api.post.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('.page-header .btn-primary').trigger('click')
    const textareas = wrapper.findAll('textarea')
    await textareas[0].setValue('/usr/bin/chromium-browser')
    await textareas[1].setValue('--no-sandbox\n--remote-debugging-port=9222')
    await wrapper.find('form').trigger('submit.prevent')

    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      spec: expect.objectContaining({
        command: ['/usr/bin/chromium-browser'],
        args: ['--no-sandbox', '--remote-debugging-port=9222'],
      }),
    }))
  })

  it('creates, edits, and unbinds individual endpoints', async () => {
    const { api } = await import('../api/index.js')
    api.post.mockReset()
    api.put.mockReset()
    api.delete.mockReset()
    api.post.mockResolvedValue({ id: 9, domain_id: 4, domain: 'api.example.com', path: '/v2', service_port: 80, tls_enabled: true })
    api.put.mockResolvedValue({ id: 7, domain_id: 4, domain: 'api.example.com', path: '/v3', service_port: 80, tls_enabled: false })
    api.delete.mockResolvedValue({ id: 7 })
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('.detail-section .section-heading .btn').trigger('click')
    await wrapper.find('.modal .form-select').setValue('4')
    await wrapper.find('.modal .form-input').setValue('/v2')
    await wrapper.find('.modal form').trigger('submit.prevent')
    expect(api.post).toHaveBeenCalledWith('/applications/1/endpoints', { domain_id: 4, path: '/v2', tls_enabled: true })

    await wrapper.find('.endpoint-row .btn').trigger('click')
    await wrapper.find('.modal .form-input').setValue('/v3')
    await wrapper.find('.modal .check-row input').setValue(false)
    await wrapper.find('.modal form').trigger('submit.prevent')
    expect(api.put).toHaveBeenCalledWith('/applications/1/endpoints/7', { domain_id: 4, path: '/v3', tls_enabled: false })

    await wrapper.find('.endpoint-row .btn-danger').trigger('click')
    expect(api.delete).toHaveBeenCalledWith('/applications/1/endpoints/7')
  })
})
