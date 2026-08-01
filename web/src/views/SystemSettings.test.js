import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import SystemSettings from './SystemSettings.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/platform/status') return Promise.resolve({ webhook_configured: true, image_prefix: 'registry.example.com/cylism-manager', deployment: { image: 'registry.example.com/cylism-manager@sha256:abc', ready_replicas: 1 }, releases: [{ id: 3, status: 'succeeded', image: 'registry.example.com/cylism-manager@sha256:abc' }] })
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
    expect(wrapper.text()).toContain('registry.example.com/cylism-manager@sha256:abc')

    const imageInput = wrapper.get('[data-testid="platform-manual-image"]')
    await imageInput.setValue('registry.example.com/cylism-manager@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')
    await wrapper.get('[data-testid="platform-manual-update"]').trigger('click')
    expect(wrapper.text()).toContain('平台更新已提交')

    await wrapper.get('[data-testid="generate-platform-webhook-secret"]').trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('generated-secret')
  })
})
