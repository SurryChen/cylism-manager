import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PodTerminal from './PodTerminal.vue'

vi.mock('../../utils/terminalRuntime.js', () => ({
  loadTerminalRuntime: vi.fn(() => Promise.reject(new Error('终端组件加载失败'))),
}))

describe('PodTerminal', () => {
  it('lets users choose a container before connecting', async () => {
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'project-demo', name: 'api-123', containers: ['api', 'sidecar'] } },
      global: { stubs: { Teleport: true } },
    })

    expect(wrapper.text()).toContain('该 Pod 包含多个容器')
    expect(wrapper.findAll('select option')).toHaveLength(3)
    expect(wrapper.find('button.btn-primary').element.disabled).toBe(true)

    await wrapper.get('select').setValue('sidecar')
    expect(wrapper.find('button.btn-primary').element.disabled).toBe(false)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('终端组件加载失败')
  })

  it('emits close and cleans up when the terminal is closed', async () => {
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'project-demo', name: 'api-123', containers: ['api', 'sidecar'] } },
      global: { stubs: { Teleport: true } },
    })

    await wrapper.get('button[title="关闭终端"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
