import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChatDrawer from './ChatDrawer.vue'

const apiMocks = vi.hoisted(() => ({ chatSessions: vi.fn(), chatMessages: vi.fn(), chatStream: vi.fn() }))
vi.mock('../api/index.js', () => apiMocks)

const runtime = () => ({ id: 1, name: 'nanobot-main', image: 'cylism-nanobot-runtime:0.3.0', runtime_version: '0.3.0' })

function mountDrawer() {
  return mount(ChatDrawer, { props: { modelValue: true, runtime: runtime() }, global: { stubs: { Teleport: true } } })
}

describe('ChatDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.chatSessions.mockResolvedValue([{ id: 'abc', title: 'Hello', updated_at: '2026-01-01T00:00:00Z' }])
    apiMocks.chatMessages.mockResolvedValue({
      id: 'abc',
      title: 'Hello',
      messages: [
        { role: 'user', content: 'hi', created_at: '2026-01-01T00:00:00Z' },
        { role: 'assistant', content: 'hello', created_at: '2026-01-01T00:00:01Z' },
      ],
    })
    apiMocks.chatStream.mockReturnValue(Promise.resolve())
  })

  it('loads sessions and history when opened', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    expect(apiMocks.chatSessions).toHaveBeenCalledWith(1)
    expect(apiMocks.chatMessages).toHaveBeenCalledWith(1, 'abc')
    expect(wrapper.text()).toContain('hi')
    expect(wrapper.text()).toContain('hello')
  })

  it('streams deltas into the assistant message', async () => {
    let onEvent
    apiMocks.chatStream.mockImplementation((_id, _body, handlers) => {
      onEvent = handlers.onEvent
      return Promise.resolve()
    })
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-input').setValue('你好')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    expect(apiMocks.chatStream).toHaveBeenCalledWith(1, expect.objectContaining({ message: '你好' }), expect.any(Object))
    onEvent({ type: 'delta', content: '正' })
    onEvent({ type: 'delta', content: '常' })
    await flushPromises()
    expect(wrapper.text()).toContain('正常')
  })

  it('stops streaming via the stop button', async () => {
    const abort = vi.fn()
    apiMocks.chatStream.mockReturnValue(Object.assign(new Promise(() => {}), { abort }))
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-input').setValue('你好')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '停止').trigger('click')
    expect(abort).toHaveBeenCalled()
  })

  it('shows an error when sessions cannot be loaded', async () => {
    apiMocks.chatSessions.mockRejectedValue(new Error('集群未连接'))
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.text()).toContain('集群未连接')
  })
})
