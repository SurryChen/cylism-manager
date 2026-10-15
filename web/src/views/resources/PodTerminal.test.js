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
})
