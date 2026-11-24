import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PodTerminal from './PodTerminal.vue'
import { loadTerminalRuntime } from '../../utils/terminalRuntime.js'

vi.mock('../../utils/terminalRuntime.js', () => ({
  loadTerminalRuntime: vi.fn(() => Promise.reject(new Error('终端组件加载失败'))),
}))

afterEach(() => { vi.unstubAllGlobals(); localStorage.removeItem('access_token') })

describe('PodTerminal', () => {
  it('lets users choose a container before connecting', async () => {
    document.body.style.overflow = 'auto'
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'project-demo', name: 'api-123', containers: ['api', 'sidecar'] } },
      global: { stubs: { Teleport: true } },
    })

    expect(document.body.style.overflow).toBe('hidden')
    expect(wrapper.text()).toContain('该 Pod 包含多个容器')
    expect(wrapper.findAll('select option')).toHaveLength(3)
    expect(wrapper.find('button.btn-primary').element.disabled).toBe(true)

    await wrapper.get('select').setValue('sidecar')
    expect(wrapper.find('button.btn-primary').element.disabled).toBe(false)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('终端组件加载失败')
    wrapper.unmount()
    expect(document.body.style.overflow).toBe('auto')
  })

  it('emits close and cleans up when the terminal is closed', async () => {
    document.body.style.overflow = 'auto'
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'project-demo', name: 'api-123', containers: ['api', 'sidecar'] } },
      global: { stubs: { Teleport: true } },
    })

    await wrapper.get('button[title="关闭终端"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(document.body.style.overflow).toBe('auto')
    wrapper.unmount()
  })

  it('closes the connected socket, terminal and resize observer', async () => {
    const sockets = [], close = vi.fn(), dispose = vi.fn(), disconnect = vi.fn()
    vi.stubGlobal('WebSocket', class { constructor() { sockets.push(this) } close = close })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect = disconnect })
    loadTerminalRuntime.mockResolvedValueOnce({ Terminal: class { loadAddon() {} open() {} dispose = dispose }, FitAddon: class { fit() {} }, installTerminalClipboard() {} })
    const wrapper = mount(PodTerminal, { props: { pod: { namespace: 'RDS', name: 'dts', containers: ['mysql'] }, databaseSession: { instance_id: 1, id: 'session' } }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('button[aria-label="关闭终端"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(close).toHaveBeenCalledOnce()
    expect(dispose).toHaveBeenCalledOnce()
    expect(disconnect).toHaveBeenCalledOnce()
    expect(sockets[0].onmessage).toBeNull()
    wrapper.unmount()
  })

  it('connects a MySQL session through its cluster-specific endpoint', async () => {
    const sockets = []
    class FakeWebSocket {
      constructor(url) { this.url = url; sockets.push(this) }
      close() {}
    }
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    loadTerminalRuntime.mockResolvedValueOnce({
      Terminal: class { loadAddon() {} open() {} dispose() {} },
      FitAddon: class { fit() {} },
      installTerminalClipboard: () => {},
    })
    localStorage.setItem('access_token', 'test-token')
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'cylism-databases', name: 'mysql-1-0', containers: ['mysql'] }, mysqlClusterId: '1' },
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('数据库终端')
    expect(sockets).toHaveLength(1)
    expect(sockets[0].url).toContain('/api/cloud/mysql-clusters/1/pods/mysql-1-0/terminal?token=test-token')
    expect(sockets[0].url).not.toContain('container=')
    wrapper.unmount()
  })

  it('requests a fresh database session when reconnecting after cleanup', async () => {
    const sockets = []
    vi.stubGlobal('WebSocket', class {
      constructor(url) { this.url = url; sockets.push(this) }
      close() {}
    })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    loadTerminalRuntime.mockResolvedValueOnce({
      Terminal: class { loadAddon() {} open() {} dispose() {} },
      FitAddon: class { fit() {} },
      installTerminalClipboard: () => {},
    })
    const wrapper = mount(PodTerminal, {
      props: { pod: { namespace: 'RDS', name: 'dts', containers: ['mysql'] }, databaseSession: { instance_id: 2, id: 'one-use-session' } },
      global: { stubs: { Teleport: true } },
    })
    await flushPromises()
    expect(sockets[0].url).toContain('/api/cloud/databases/2/sessions/one-use-session/terminal')
    sockets[0].onclose()
    await flushPromises()
    await wrapper.get('.terminal-notice .btn-primary').trigger('click')
    expect(wrapper.emitted('reconnect')).toHaveLength(1)
    expect(sockets).toHaveLength(1)
    wrapper.unmount()
  })

  it('keeps MySQL error output visible and allows choosing another execution node', async () => {
    const sockets = [], write = vi.fn()
    vi.stubGlobal('WebSocket', class { constructor() { sockets.push(this) } close() {} })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    loadTerminalRuntime.mockResolvedValueOnce({ Terminal: class { loadAddon() {} open() {} dispose() {} write = write }, FitAddon: class { fit() {} }, installTerminalClipboard() {} })
    const wrapper = mount(PodTerminal, { props: { pod: { namespace: 'RDS', name: 'dts', containers: ['mysql'] }, databaseSession: { instance_id: 1, id: 'session' } }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    sockets[0].onmessage({ data: JSON.stringify({ type: 'status', detail: '正在拉取客户端镜像' }) })
    await flushPromises()
    expect(wrapper.text()).toContain('正在拉取客户端镜像')
    const encoded = new TextEncoder().encode('ERROR 2005 (HY000): Unknown MySQL server host')
    const output = new ArrayBuffer(encoded.length)
    new Uint8Array(output).set(encoded)
    sockets[0].onmessage({ data: output })
    sockets[0].onmessage({ data: JSON.stringify({ type: 'error', detail: 'command terminated with exit code 1' }) })
    await flushPromises()
    expect(write).toHaveBeenCalled()
    expect(wrapper.get('.terminal-container').isVisible()).toBe(true)
    expect(wrapper.text()).toContain('下方保留完整终端输出')
    await wrapper.get('.terminal-notice .btn:not(.btn-primary)').trigger('click')
    expect(wrapper.emitted('configure')).toHaveLength(1)
    wrapper.unmount()
  })
})
