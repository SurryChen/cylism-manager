import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Sites from './Sites.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
      if (url.includes('ingress-controller')) {
        return Promise.resolve({ type: 'Traefik', version: '2.10.7', running: true, crd: true, namespace: 'kube-system' })
      }
      if (url.includes('/k8s/ingresses')) {
        return Promise.resolve([
          { name: 'web-ingress', namespace: 'default', hosts: ['example.com'], paths: ['/ -> web-svc:80'], tls: ['tls-cert'], controller: 'traefik', age: '2d' }
        ])
      }
      // /routes
      return Promise.resolve([
        { name: 'my-route', namespace: 'default', domain: 'Host(`example.com`)', tls: 'true', created_at: '2024-01-01 00:00' }
      ])
    }),
    post: vi.fn().mockResolvedValue({ message: 'ok' }),
    delete: vi.fn().mockResolvedValue({ message: 'ok' }),
  }
}))

beforeEach(() => {
  document.body.innerHTML = ''
  sessionStorage.clear()
})

describe('Sites view with dual tabs', () => {
  it('renders Traefik controller banner when detected', async () => {
    const wrapper = mount(Sites, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const banner = wrapper.find('.controller-banner')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('Traefik')
    expect(banner.classes()).toContain('controller-ok')
  })

  it('renders two tab buttons: IngressRoute and Ingress', () => {
    const wrapper = mount(Sites, {
      global: { stubs: { RouterLink: true } }
    })
    const tabs = wrapper.findAll('.tab-btn')
    expect(tabs).toHaveLength(2)
    expect(tabs[0].text()).toBe('IngressRoute')
    expect(tabs[1].text()).toBe('标准 Ingress')
  })

  it('switches to standard Ingress tab', async () => {
    const wrapper = mount(Sites, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    const ingressTab = wrapper.findAll('.tab-btn')[1]
    await ingressTab.trigger('click')
    await nextTick()
    await new Promise(r => setTimeout(r, 100))

    expect(wrapper.text()).toContain('web-ingress')
  })

  it('shows IngressRoute list content on default tab', async () => {
    const wrapper = mount(Sites, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()

    expect(wrapper.text()).toContain('my-route')
  })

  it('renders the cached controller status while a refresh is pending', async () => {
    const { api } = await import('../api/index.js')
    sessionStorage.setItem('cylism.ingress-controller.status', JSON.stringify({
      status: { type: 'Traefik', version: '3.7.4', running: true, crd: true, namespace: 'kube-system' },
      expiresAt: Date.now() - 1
    }))
    api.get.mockImplementation(url => {
      if (url.includes('ingress-controller')) return new Promise(() => {})
      return Promise.resolve([])
    })

    const wrapper = mount(Sites)
    await nextTick()

    expect(wrapper.find('.controller-banner').text()).toContain('Traefik 3.7.4')
  })
})
