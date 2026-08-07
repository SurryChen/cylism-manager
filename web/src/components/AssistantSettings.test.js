import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import AssistantSettings from './AssistantSettings.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/assistant/status') return Promise.resolve({ configured: true, default_provider_id: '1', runtime_status: { state: 'ready', message: '智能助手 Runtime 已就绪', ready_replicas: 1, node_name: 'worker-a', storage: '1Gi', model: 'gpt-4.1-mini' }, migration: { status: 'copying', bytes_copied: 1024 } })
      if (path === '/assistant/providers') return Promise.resolve({ providers: [{ id: 1, name: 'Responses', model: 'gpt-4.1-mini', enabled: true }], default_provider_id: '1' })
      if (path === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true }])
      return Promise.resolve({})
    }),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('AssistantSettings', () => {
  it('shows Kubernetes readiness separately from the saved model provider', async () => {
    const wrapper = mount(AssistantSettings)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(wrapper.text()).toContain('模型已配置')
    expect(wrapper.text()).toContain('Runtime 已就绪')
    expect(wrapper.text()).toContain('1 / 1')
    expect(wrapper.text()).toContain('worker-a')
    expect(wrapper.text()).toContain('gpt-4.1-mini')
    expect(wrapper.text()).toContain('正在复制审计数据')
    expect(wrapper.text()).toContain('Runtime 存储迁移')
    expect(wrapper.findAll('.assistant-install select')[1].element.value).toBe('worker-a')
    wrapper.unmount()
  })

  it('identifies an old audit PVC as a migration source and preselects its available node', async () => {
    api.get.mockImplementation(path => {
      if (path === '/assistant/status') return Promise.resolve({ configured: true, default_provider_id: '1', runtime_status: { state: 'not_installed', message: '尚未安装智能助手 Runtime' }, legacy_runtime_status: { state: 'not_installed', namespace: 'default', pvc_name: 'cylism-ops-agent-audit', node_name: 'worker-a', storage: '1Gi' } })
      if (path === '/assistant/providers') return Promise.resolve({ providers: [{ id: 1, name: 'Responses', model: 'gpt-4.1-mini', enabled: true }], default_provider_id: '1' })
      if (path === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true }])
      return Promise.resolve({})
    })
    const wrapper = mount(AssistantSettings)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(wrapper.text()).toContain('发现待迁移的旧 Runtime 审计存储')
    expect(wrapper.text()).toContain('default / cylism-ops-agent-audit')
    expect(wrapper.find('.assistant-install select').element.value).toBe('1')
    expect(wrapper.findAll('.assistant-install select')[1].element.value).toBe('worker-a')
    expect(wrapper.text()).toContain('迁移并部署 Runtime')
    wrapper.unmount()
  })

  it('uninstalls the Runtime and its PVC', async () => {
    api.delete.mockResolvedValue({ message: '智能助手 Runtime 和审计 PVC 已卸载' })
    api.get.mockImplementation(path => {
      if (path === '/assistant/status') return Promise.resolve({ configured: true, default_provider_id: '1', runtime_status: { state: 'ready', message: '智能助手 Runtime 已就绪', ready_replicas: 1 } })
      if (path === '/assistant/providers') return Promise.resolve({ providers: [{ id: 1, name: 'Responses', model: 'gpt-4.1-mini', enabled: true }], default_provider_id: '1' })
      if (path === '/nodes') return Promise.resolve([{ name: 'worker-a', ready: true }])
      return Promise.resolve({})
    })
    const wrapper = mount(AssistantSettings)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    window.confirm = vi.fn(() => true)
    await wrapper.get('button[aria-label="卸载 Runtime 和 PVC"]').trigger('click')
    await nextTick()

    expect(api.delete).toHaveBeenCalledWith('/assistant')
    expect(wrapper.text()).toContain('智能助手 Runtime 和审计 PVC 已卸载')
    wrapper.unmount()
  })

  it('probes the Responses API without saving the provider', async () => {
    api.post.mockResolvedValue({ message: '模型 API 连接成功' })
    const wrapper = mount(AssistantSettings)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    await wrapper.get('.assistant-provider-probe').trigger('click')
    await nextTick()

    expect(api.post).toHaveBeenCalledWith('/assistant/providers/test', expect.objectContaining({ provider_type: 'openai_responses', model: '' }))
    expect(wrapper.text()).toContain('模型 API 连接成功')
    wrapper.unmount()
  })
})
