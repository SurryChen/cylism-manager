import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Servers from './Servers.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.startsWith('/servers')) return Promise.resolve([{ id: 1, name: 'test-srv', host: '10.0.0.1', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, tailscale_ip: '', tailscale_online: false, cluster_role: '', k8s_node_name: '' }])
      if (url.startsWith('/nodes')) return Promise.resolve([])
      return Promise.resolve({})
    }),
    post: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({ message: 'ok' })
  }
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('Servers view', () => {
  it('renders two tabs: 服务器列表 and 集群节点', () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    const tabs = wrapper.findAll('.tab-btn')
    expect(tabs).toHaveLength(2)
    expect(tabs[0].text()).toBe('服务器列表')
    expect(tabs[1].text()).toBe('集群节点')
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

  it('shows empty nodes tab', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await nextTick()
    const tabs = wrapper.findAll('.tab-btn')
    await tabs[1].trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('暂无 K8s 节点')
  })
})
