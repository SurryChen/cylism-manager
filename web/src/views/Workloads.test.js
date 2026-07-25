import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Workloads from './Workloads.vue'

vi.mock('../api/index.js', () => {
  return {
    api: {
      get: vi.fn().mockImplementation((url) => {
        if (url.includes('/deployments')) return Promise.resolve([])
        if (url.includes('/statefulsets')) return Promise.resolve([])
        if (url.includes('/daemonsets')) return Promise.resolve([])
        return Promise.resolve([])
      }),
      patch: vi.fn().mockResolvedValue({ message: 'ok' }),
      post: vi.fn().mockResolvedValue({ message: 'ok' }),
    }
  }
})

function flush() {
  return new Promise(r => setTimeout(r, 200))
}

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('Workloads view', () => {
  it('renders three tab buttons', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const tabs = wrapper.findAll('.tab-btn')
    expect(tabs).toHaveLength(3)
    expect(tabs[0].text()).toBe('Deployments')
    expect(tabs[1].text()).toBe('StatefulSets')
    expect(tabs[2].text()).toBe('DaemonSets')
  })

  it('shows empty state for deployments when no data', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    expect(wrapper.text()).toContain('暂无 Deployment')
  })

  it('switches to StatefulSets tab', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const stsTab = wrapper.findAll('.tab-btn')[1]
    await stsTab.trigger('click')
    await nextTick()
    await flush()
    expect(wrapper.text()).toContain('暂无 StatefulSet')
  })

  it('switches to DaemonSets tab', async () => {
    const wrapper = mount(Workloads, {
      global: { stubs: { RouterLink: true } }
    })
    await flush()
    const dsTab = wrapper.findAll('.tab-btn')[2]
    await dsTab.trigger('click')
    await nextTick()
    await flush()
    expect(wrapper.text()).toContain('暂无 DaemonSet')
  })
})
