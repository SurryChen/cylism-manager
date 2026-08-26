import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import SystemSettings from './SystemSettings.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/platform/status') return Promise.resolve({ webhook_configured: true, image_prefix: 'registry.example.com/cylism-manager', deployment: { image: 'registry.example.com/cylism-manager:latest', ready_replicas: 1 }, releases: [{ id: 3, source: 'github', status: 'succeeded', image: 'registry.example.com/cylism-manager:latest', commit_sha: 'aabbccddeeff00112233445566778899', created_at: '2026-08-01T12:00:00Z' }] })
      if (path === '/platform/endpoint') return Promise.resolve({ endpoint: { hostname: 'console.example.com', certificate_name: 'console-example-com', enabled: true }, url: 'https://console.example.com', state: 'ready', ingress_ready: true, certificate: { name: 'console-example-com', status: 'Ready' } })
      if (path === '/certs') return Promise.resolve([{ name: 'console-example-com', namespace: 'default', status: 'Ready', domains: ['console.example.com'] }])
      return Promise.resolve({ initialized: true, ip: '100.88.0.1', online: true })
    }),
    post: vi.fn().mockResolvedValue({ secret: 'generated-secret' }),
    put: vi.fn().mockResolvedValue({}),
  },
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('SystemSettings view', () => {
  it('shows tailscale summary and setup entry', async () => {
    const wrapper = mount(SystemSettings, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="to"><slot /></a>',
          },
        },
      },
    })
    await new Promise(r => setTimeout(r, 50))
    await nextTick()

    expect(wrapper.text()).toContain('系统设置')
    expect(wrapper.text()).toContain('100.88.0.1')
    expect(wrapper.text()).toContain('不再单独占一个基础设施页面')
    expect(wrapper.text()).toContain('平台自更新')
    expect(wrapper.text()).toContain('registry.example.com/cylism-manager:latest')
    expect(wrapper.text()).toContain('最近一次自动更新')
    expect(wrapper.text()).toContain('提交 aabbccddeeff')
    expect(wrapper.text()).toContain('平台管理入口')
    expect(wrapper.text()).toContain('https://console.example.com')
    expect(wrapper.text()).toContain('console-example-com')

    const imageInput = wrapper.get('[data-testid="platform-manual-image"]')
    await imageInput.setValue('registry.example.com/cylism-manager:latest')
    await wrapper.get('[data-testid="platform-manual-update"]').trigger('click')
    expect(wrapper.text()).toContain('平台更新已提交')

    await wrapper.get('[data-testid="generate-platform-webhook-secret"]').trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('generated-secret')
  })
})
