import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RegistryProxyRelayPanel from './RegistryProxyRelayPanel.vue'
import { api } from '../../api/index.js'

vi.mock('../../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('RegistryProxyRelayPanel', () => {
  it('shows relay addresses and links to the existing cluster mirror editor', async () => {
    api.get.mockResolvedValue([{ id: 4, server_id: 8, listen_ip: '100.67.232.15', listen_port: 31501, status: 'ready' }])
    const wrapper = mount(RegistryProxyRelayPanel, {
      props: { proxy: { id: 2, name: '镜像代理', registry: 'registry.k8s.io', cluster_id: 7 }, servers: [{ id: 8, name: '中转节点', host: '100.67.232.15' }] },
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
    })
    await vi.waitFor(() => expect(document.body.querySelector('.relay-endpoint')?.textContent).toContain('http://100.67.232.15:31501'))

    const modal = document.body.querySelector('.relay-modal')
    expect(modal.querySelector('.relay-submit').disabled).toBe(false)
    expect(modal.querySelector('.relay-actions button').disabled).toBe(false)
    expect(modal.textContent).not.toContain('目标节点')
    expect(modal.textContent).not.toContain('切换入口')
    expect(modal.querySelector('a[href="/cluster/7?tab=registry-mirrors"]')).not.toBeNull()
    expect(api.get).toHaveBeenCalledWith('/registry-proxies/2/relays')
    expect(api.get).not.toHaveBeenCalledWith('/registry-proxies/2/node-routes')
    wrapper.unmount()
  })

  it('submits the deployment form when the relay is idle', async () => {
    api.get.mockResolvedValue([])
    api.post.mockResolvedValue({})
    const wrapper = mount(RegistryProxyRelayPanel, {
      props: { proxy: { id: 2, name: '镜像代理', registry: 'ghcr.io', cluster_id: 7 }, servers: [{ id: 8, name: '中转节点', host: '100.67.232.15' }] },
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
    })
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledWith('/registry-proxies/2/relays'))
    const form = document.body.querySelector('.relay-deploy-form')
    const server = form.querySelector('select')
    server.value = '8'
    server.dispatchEvent(new Event('change', { bubbles: true }))
    for (const [selector, value] of [['#relay-listen-ip', '100.67.232.15'], ['#relay-listen-port', '31501']]) {
      const input = form.querySelector(selector)
      input.value = value
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
    await wrapper.vm.$nextTick()
    expect(form.querySelector('.relay-submit').disabled).toBe(false)
    form.querySelector('.relay-submit').click()
    await vi.waitFor(() => expect(api.post).toHaveBeenCalledWith('/registry-proxies/2/relays', { server_id: 8, listen_ip: '100.67.232.15', listen_port: 31501 }))
    wrapper.unmount()
  })
})
