import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ServerK3sCleanupModal from './ServerK3sCleanupModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post } }))
const source = { id: 10, node_name: 'worker', node_uid: 'uid-1' }
const server = { id: 4, name: '工作节点', k3s_removal_task_id: 10 }
const render = () => mount(ServerK3sCleanupModal, { props: { server } })
const confirm = wrapper => wrapper.findAll('button').find(button => button.text() === '确认卸载并清理')

describe('ServerK3sCleanupModal', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); get.mockResolvedValue({ source, task: null, logs: [] }) })
  afterEach(() => { vi.useRealTimers() })

  it('requires the original node name and follows the task through completion', async () => {
    vi.useFakeTimers()
    const wrapper = render()
    await flushPromises()
    expect(get).toHaveBeenCalledWith('/servers/4/k3s-cleanup', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    await wrapper.get('#cleanup-node-name').setValue('worker')
    expect(confirm(wrapper).attributes('disabled')).toBeUndefined()
    post.mockResolvedValue({ id: 11, status: 'running' })
    get.mockResolvedValueOnce({ source, task: { id: 11, status: 'running' }, logs: [{ id: 1, message: '核对主机身份' }] })
    await confirm(wrapper).trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/servers/4/k3s-cleanup', { removal_task_id: 10, confirm_node_name: 'worker' })
    expect(wrapper.text()).toContain('清理中')
    expect(wrapper.text()).toContain('核对主机身份')
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    get.mockResolvedValue({ source: null, task: { id: 11, status: 'success' }, logs: [{ id: 2, message: '服务及本地数据已清理' }] })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.text()).toContain('清理完成')
    expect(wrapper.emitted('completed')).toHaveLength(1)
    expect(confirm(wrapper)).toBeUndefined()
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    expect(get).toHaveBeenCalledTimes(3)
  })

  it('restores a failed task and keeps typed confirmation after submit failure', async () => {
    get.mockResolvedValue({ source, task: { id: 11, status: 'failed' }, logs: [{ id: 1, message: '主机身份核对失败' }] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('清理失败，可核对后重试')
    await wrapper.get('#cleanup-node-name').setValue('worker')
    post.mockRejectedValue(new Error('服务器已重新绑定'))
    await confirm(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ErrorNoticeModal).props('message')).toBe('服务器已重新绑定')
    expect(wrapper.get('#cleanup-node-name').element.value).toBe('worker')
    expect(wrapper.emitted('completed')).toBeUndefined()
    wrapper.unmount()
  })

  it('discards stale responses after switching servers', async () => {
    let resolveOld
    get.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    await wrapper.setProps({ server: { id: 5 } })
    await flushPromises()
    resolveOld({ source: { ...source, node_name: 'stale-worker' }, task: null })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-worker')
    expect(get).toHaveBeenLastCalledWith('/servers/5/k3s-cleanup', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    wrapper.unmount()
  })

  it('keeps destructive actions disabled on a failed status read', async () => {
    get.mockRejectedValue(new Error('读取失败'))
    const wrapper = render()
    await flushPromises()
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.findComponent(ErrorNoticeModal).props('message')).toBe('读取失败')
    expect(post).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('requires a fresh successful status check after a refresh fails', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#cleanup-node-name').setValue('worker')
    expect(confirm(wrapper).attributes('disabled')).toBeUndefined()
    get.mockRejectedValueOnce(new Error('读取失败'))
    await wrapper.findAll('button').find(button => button.text() === '刷新状态').trigger('click')
    await flushPromises()
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    expect(post).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
