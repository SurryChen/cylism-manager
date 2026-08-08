import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RuntimeManagement from './RuntimeManagement.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('../api/index.js', () => ({ api: apiMocks }))

const runtime = () => ({ id: 1, name: 'nanobot-main', runtime_type: 'nanobot', image: 'example/nanobot:latest', namespace: 'cylism-assistant', status: 'ready', health_status: 'ready', health_detail: 'Runtime 健康检查通过', port: 8080, health_path: '/health', pvc_name: 'nanobot-main-data', storage: '10Gi', model_name: 'qwen-max', api_style: 'responses', api_key_configured: true })

describe('RuntimeManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
    apiMocks.get.mockResolvedValue([runtime()])
    apiMocks.post.mockResolvedValue(runtime())
    apiMocks.put.mockResolvedValue(runtime())
  })

  it('lists runtime instances and exposes deployment actions', async () => {
    const wrapper = mount(RuntimeManagement)
    await flushPromises()
    expect(wrapper.text()).toContain('nanobot-main')
    expect(wrapper.text()).toContain('cylism-assistant')
    await wrapper.get('.runtime-item').trigger('click')
    expect(wrapper.text()).toContain('部署或更新')
    await wrapper.findAll('.runtime-detail .btn').find(button => button.text() === '健康检查').trigger('click')
    expect(apiMocks.post).toHaveBeenCalledWith('/runtimes/1/health-check')
  })

  it('creates a runtime with a model connection', async () => {
    const wrapper = mount(RuntimeManagement, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('[data-testid="runtime-create"]').trigger('click')
    expect(wrapper.find('.runtime-create-modal').exists()).toBe(true)
    expect(wrapper.get('.runtime-detail-card').text()).toContain('选择一个助手实例查看详情')
    await wrapper.get('input[placeholder="nanobot-main"]').setValue('nanobot-prod')
    await wrapper.get('input[placeholder="托管模式填写镜像地址"]').setValue('example/nanobot:v1')
    await wrapper.get('input[placeholder="模型名称"]').setValue('qwen-max')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
    expect(apiMocks.post).toHaveBeenCalledWith('/runtimes', expect.objectContaining({ name: 'nanobot-prod', image: 'example/nanobot:v1', model_name: 'qwen-max', port: 8900, health_path: '/health' }))
  })

  it('uninstalls a runtime and keeps PVC by default', async () => {
    const wrapper = mount(RuntimeManagement, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.runtime-item').trigger('click')
    await wrapper.findAll('.runtime-detail .btn').find(button => button.text() === '卸载').trigger('click')
    expect(wrapper.find('.runtime-uninstall-modal').exists()).toBe(true)
    expect(wrapper.find('.runtime-uninstall-modal [data-testid="runtime-delete-data"]').exists()).toBe(true)
    await wrapper.findAll('.runtime-uninstall-modal .modal-actions .btn').find(button => button.text() === '确认卸载').trigger('click')
    expect(apiMocks.post).toHaveBeenCalledWith('/runtimes/1/uninstall')
  })

  it('deletes PVC and memory data only when the uninstall option is checked', async () => {
    const wrapper = mount(RuntimeManagement, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.runtime-item').trigger('click')
    await wrapper.findAll('.runtime-detail .btn').find(button => button.text() === '卸载').trigger('click')
    await wrapper.get('.runtime-uninstall-modal [data-testid="runtime-delete-data"]').setValue(true)
    expect(wrapper.text()).toContain('将永久删除 Runtime 的会话')
    await wrapper.findAll('.runtime-uninstall-modal .modal-actions .btn').find(button => button.text() === '确认卸载').trigger('click')
    expect(apiMocks.post).toHaveBeenCalledWith('/runtimes/1/uninstall?delete_data=true')
  })

  it('shows a modal after a successful action', async () => {
    const wrapper = mount(RuntimeManagement, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.runtime-item').trigger('click')
    await wrapper.findAll('.runtime-detail .btn').find(button => button.text() === '健康检查').trigger('click')
    expect(wrapper.find('.runtime-notice-modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('操作成功')
    expect(wrapper.text()).toContain('健康检查已完成')
    await wrapper.get('.runtime-notice-modal .modal-actions .btn').trigger('click')
    expect(wrapper.find('.runtime-notice-modal').exists()).toBe(false)
  })

  it('shows an error modal when an action fails', async () => {
    apiMocks.post.mockRejectedValueOnce(new Error('部署失败：权限修复超时'))
    const wrapper = mount(RuntimeManagement, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.runtime-item').trigger('click')
    await wrapper.findAll('.runtime-detail .btn').find(button => button.text() === '部署或更新').trigger('click')
    expect(wrapper.find('.runtime-notice-modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('操作失败')
    expect(wrapper.text()).toContain('部署失败：权限修复超时')
  })
})
