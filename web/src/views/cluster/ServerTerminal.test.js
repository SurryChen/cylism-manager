import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import ServerTerminal from './ServerTerminal.vue'

vi.mock('../../utils/terminalRuntime.js', () => ({
  loadTerminalRuntime: vi.fn(() => Promise.reject(new Error('终端组件加载失败'))),
}))

describe('ServerTerminal', () => {
  it('shows an isolated loading failure and emits close', async () => {
    document.body.style.overflow = 'auto'
    const wrapper = mount(ServerTerminal, {
      props: { server: { id: 1, name: 'test-srv', host: '10.0.0.1' } },
      global: { stubs: { Teleport: true } },
    })
    expect(document.body.style.overflow).toBe('hidden')
    await nextTick()
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(wrapper.text()).toContain('终端组件加载失败')
    await wrapper.get('button[title="关闭"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(document.body.style.overflow).toBe('auto')
    wrapper.unmount()
  })
})
