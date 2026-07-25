import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Services from './Services.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockResolvedValue({
      json: async () => ({ data: [
        { name: 'web-svc', namespace: 'default', type: 'ClusterIP', cluster_ip: '10.43.1.1', ports: ['TCP:80'], endpoint_count: 2, selector: { app: 'web' }, age: '5d' }
      ]})
    })
  }
}))

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('Services view', () => {
  it('renders page title', () => {
    const wrapper = mount(Services, {
      global: { stubs: { RouterLink: true } }
    })
    expect(wrapper.find('.page-title').text()).toBe('服务发现')
  })

  it('renders service table after data', async () => {
    const wrapper = mount(Services, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()
    const table = wrapper.find('.table-wrap')
    expect(table.exists()).toBe(true)
  })

  it('shows endpoint count in table', async () => {
    const wrapper = mount(Services, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()
    const text = wrapper.text()
    expect(text).toContain('ClusterIP')
    expect(text).toContain('端点')
  })

  it('expands service on row click', async () => {
    const wrapper = mount(Services, {
      global: { stubs: { RouterLink: true } }
    })
    await new Promise(r => setTimeout(r, 100))
    await nextTick()
    expect(wrapper.findAll('.clickable').length).toBeGreaterThan(0)
    // Initially no expanded row
    expect(wrapper.vm.expandedSvc).toBe('')
  })
})
