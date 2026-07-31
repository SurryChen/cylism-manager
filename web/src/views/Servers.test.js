import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Servers from './Servers.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.startsWith('/servers')) return Promise.resolve([
        { id: 1, name: 'test-srv', host: '10.0.0.1', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: '', k8s_node_name: '' },
        { id: 2, name: 'cluster-srv', host: '10.0.0.2', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: 'worker', k8s_node_name: 'worker-a' },
      ])
      return Promise.resolve({})
    }),
    post: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({ message: 'ok' })
  }
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('Servers view', () => {
  it('renders a server registry page without node tabs', () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    expect(wrapper.text()).toContain('服务器')
    expect(wrapper.text()).toContain('集群节点已经拆分到“集群节点”页面')
    expect(wrapper.findAll('.tab-btn')).toHaveLength(0)
  })

  it('shows server table with new columns', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()
    const text = wrapper.text()
    expect(text).toContain('test-srv')
    expect(text).toContain('10.0.0.1')
    expect(text).toContain('root')
    expect(text).toContain('密码')
    expect(text).toContain('未加入')
  })

  it('shows add server modal on button click', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()
    const btn = wrapper.find('.btn-primary')
    await btn.trigger('click')
    await nextTick()
    expect(wrapper.find('.modal').exists()).toBe(true)
  })

  it('manually unbinds a server without deleting its Kubernetes node', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()

    const unbind = wrapper.findAll('button').find(button => button.text() === '解除绑定')
    await unbind.trigger('click')

    expect(confirm).toHaveBeenCalled()
    expect(api.post).toHaveBeenCalledWith('/servers/2/unbind')
    confirm.mockRestore()
  })

  it('keeps the SSH terminal open when the overlay is clicked during text selection', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()

    const terminal = wrapper.findAll('button').find(button => button.text() === '💻')
    await terminal.trigger('click')
    await nextTick()
    const overlay = document.querySelector('.terminal-overlay')
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(document.querySelector('.terminal-overlay')).not.toBeNull()
    wrapper.unmount()
  })
})
