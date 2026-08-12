import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChatDrawer from './ChatDrawer.vue'

const apiMocks = vi.hoisted(() => ({ chatSessions: vi.fn(), chatMessages: vi.fn(), chatStream: vi.fn(), agentOperations: vi.fn(), resolveAgentOperation: vi.fn() }))
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
    apiMocks.agentOperations.mockResolvedValue([])
  })

  it('loads sessions and history when opened', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    expect(apiMocks.chatSessions).toHaveBeenCalledWith(1)
    expect(apiMocks.chatMessages).toHaveBeenCalledWith(1, 'abc')
    expect(wrapper.text()).toContain('hi')
    expect(wrapper.text()).toContain('hello')
  })

  it('shows and resolves pending operations in the chat approval panel', async () => {
    apiMocks.agentOperations.mockResolvedValue([{ operation_id: 'op_1', status: 'pending_approval', summary: 'scale operations/api to 3 replicas', created_at: '2026-08-12T10:00:00Z' }])
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.find('.chat-action-count').text()).toBe('1')
    await wrapper.get('[aria-label="待审批操作"]').trigger('click')
    expect(wrapper.text()).toContain('scale operations/api to 3 replicas')
    await wrapper.get('.chat-approval-actions .btn-primary').trigger('click')
    expect(apiMocks.resolveAgentOperation).toHaveBeenCalledWith('op_1', true)
  })

  it('exposes a permission entry point from the chat header', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('[aria-label="Agent 权限"]').trigger('click')
    expect(wrapper.emitted('manage-permissions')).toHaveLength(1)
  })

  it('renders assistant Markdown safely', async () => {
    apiMocks.chatMessages.mockResolvedValue({
      id: 'abc',
      title: 'Hello',
      messages: [{ role: 'assistant', content: '**bold**\n\n- item\n\n<img src=x onerror=alert(1)>' }],
    })
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.find('.chat-markdown strong').text()).toBe('bold')
    expect(wrapper.find('.chat-markdown li').text()).toBe('item')
    expect(wrapper.find('.chat-markdown img').exists()).toBe(false)
  })

  it('streams deltas into the assistant message', async () => {
    let onEvent
    apiMocks.chatStream.mockImplementation((_id, _body, handlers) => {
      onEvent = handlers.onEvent
      return Object.assign(new Promise(() => {}), { abort: vi.fn() })
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

  it('shows a thinking state before the first assistant delta', async () => {
    apiMocks.chatStream.mockImplementation(() => Object.assign(new Promise(() => {}), { abort: vi.fn() }))
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-input').setValue('你好')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    expect(wrapper.text()).toContain('正在思考...')
  })

  it('keeps interleaved streams isolated while switching sessions', async () => {
    const streams = []
    apiMocks.chatStream.mockImplementation((_id, body, handlers) => {
      const stream = Object.assign(new Promise(() => {}), { abort: vi.fn() })
      streams.push({ body, handlers, stream })
      return stream
    })
    const wrapper = mountDrawer()
    await flushPromises()

    await wrapper.get('.chat-input').setValue('会话 A')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    await wrapper.get('.chat-session-new').trigger('click')
    await wrapper.get('.chat-input').setValue('会话 B')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    expect(streams).toHaveLength(2)
    expect(streams[0].body.session_id).not.toBe(streams[1].body.session_id)

    streams[0].handlers.onEvent({ type: 'delta', content: '回复 A' })
    streams[1].handlers.onEvent({ type: 'delta', content: '回复 B' })
    await flushPromises()
    expect(wrapper.find('.chat-messages').text()).toContain('回复 B')
    expect(wrapper.find('.chat-messages').text()).not.toContain('回复 A')

    const sessionA = wrapper.findAll('.chat-session').find(button => button.text().includes('Hello'))
    await sessionA.trigger('click')
    expect(wrapper.find('.chat-messages').text()).toContain('回复 A')
    expect(wrapper.find('.chat-messages').text()).not.toContain('回复 B')
  })

  it('refreshes summaries without reloading history after completion', async () => {
    let onEvent
    apiMocks.chatStream.mockImplementation((_id, _body, handlers) => {
      onEvent = handlers.onEvent
      return Object.assign(new Promise(() => {}), { abort: vi.fn() })
    })
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-input').setValue('你好')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    onEvent({ type: 'delta', content: '好的' })
    onEvent({ type: 'done' })
    await flushPromises()
    expect(apiMocks.chatSessions).toHaveBeenCalledTimes(2)
    expect(apiMocks.chatMessages).toHaveBeenCalledTimes(1)
  })

  it('loads older history only when requested with the returned cursor', async () => {
    apiMocks.chatMessages
      .mockResolvedValueOnce({
        id: 'abc',
        title: 'Hello',
        messages: [{ role: 'assistant', content: '最新消息' }],
        has_more: true,
        next_cursor: '20',
      })
      .mockResolvedValueOnce({
        id: 'abc',
        title: 'Hello',
        messages: [{ role: 'assistant', content: '更早消息' }],
        has_more: false,
      })
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.text()).toContain('最新消息')
    await wrapper.find('.chat-history-more').trigger('click')
    await flushPromises()
    expect(apiMocks.chatMessages).toHaveBeenLastCalledWith(1, 'abc', { limit: 50, before: '20' })
    expect(wrapper.find('.chat-messages').text()).toContain('更早消息')
  })

  it('assigns a distinct session ID before sending a new conversation', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-session-new').trigger('click')
    await wrapper.get('.chat-input').setValue('新的话题')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')

    const [, body] = apiMocks.chatStream.mock.calls[0]
    expect(body.session_id).toEqual(expect.any(String))
    expect(body.session_id).not.toBe('')
    expect(body.session_id).not.toBe('default')
  })

  it('does not send when Enter is used to commit an IME composition', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    const input = wrapper.get('.chat-input')
    await input.setValue('english')

    await input.trigger('keydown', { key: 'Enter', isComposing: true })

    expect(apiMocks.chatStream).not.toHaveBeenCalled()
    expect(input.element.value).toBe('english')
  })

  it('sends only after the IME composition has ended', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    const input = wrapper.get('.chat-input')
    await input.setValue('英文')

    await input.trigger('compositionstart')
    await input.trigger('keydown', { key: 'Enter' })
    expect(apiMocks.chatStream).not.toHaveBeenCalled()

    await input.trigger('compositionend')
    await input.trigger('keydown', { key: 'Enter' })

    expect(apiMocks.chatStream).toHaveBeenCalledOnce()
    expect(apiMocks.chatStream).toHaveBeenCalledWith(1, expect.objectContaining({ message: '英文' }), expect.any(Object))
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

  it('stops only the active session while another session continues streaming', async () => {
    const streams = []
    apiMocks.chatStream.mockImplementation((_id, _body, handlers) => {
      const stream = Object.assign(new Promise(() => {}), { abort: vi.fn() })
      streams.push({ handlers, stream })
      return stream
    })
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-input').setValue('会话 A')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    await wrapper.get('.chat-session-new').trigger('click')
    await wrapper.get('.chat-input').setValue('会话 B')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')

    await wrapper.findAll('button').find(button => button.text() === '停止').trigger('click')
    expect(streams[1].stream.abort).toHaveBeenCalledOnce()
    expect(streams[0].stream.abort).not.toHaveBeenCalled()
    streams[0].handlers.onEvent({ type: 'delta', content: '仍在生成' })

    const sessionA = wrapper.findAll('.chat-session').find(button => button.text().includes('Hello'))
    await sessionA.trigger('click')
    expect(wrapper.find('.chat-messages').text()).toContain('仍在生成')
  })

  it('keeps a failed message in its session and retries with the same session ID', async () => {
    const streams = []
    apiMocks.chatStream.mockImplementation((_id, body, handlers) => {
      const stream = Object.assign(new Promise(() => {}), { abort: vi.fn() })
      streams.push({ body, handlers })
      return stream
    })
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('.chat-session-new').trigger('click')
    await wrapper.get('.chat-input').setValue('需要重试')
    await wrapper.findAll('button').find(button => button.text() === '发送').trigger('click')
    streams[0].handlers.onEvent({ type: 'error', message: '连接失败' })
    await flushPromises()
    expect(wrapper.text()).toContain('连接失败')
    await wrapper.findAll('button').find(button => button.text() === '重试').trigger('click')
    expect(streams).toHaveLength(2)
    expect(streams[1].body.session_id).toBe(streams[0].body.session_id)
    expect(streams[1].body.message).toBe('需要重试')
  })

  it('shows an error when sessions cannot be loaded', async () => {
    apiMocks.chatSessions.mockRejectedValue(new Error('集群未连接'))
    const wrapper = mountDrawer()
    await flushPromises()
    expect(wrapper.text()).toContain('集群未连接')
  })
})
