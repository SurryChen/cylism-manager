import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import SystemSettings from './SystemSettings.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockResolvedValue({ initialized: true, ip: '100.88.0.1', online: true }),
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
  })
})
