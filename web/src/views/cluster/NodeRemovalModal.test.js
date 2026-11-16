import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import NodeRemovalModal from './NodeRemovalModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post } }))
const server = { id: 4, k8s_node_name: 'worker' }
const plan = { node_name: 'worker', node_uid: 'uid-1', can_start: true, drain: { evictable: [{ namespace: 'default', name: 'web', reason: '将通过 Eviction API 迁移' }], requires_empty_dir_confirmation: [], blocked: [], skipped: [] } }
const render = () => mount(NodeRemovalModal, { props: { clusterId: '9', server } })
const confirm = wrapper => wrapper.findAll('button').find(button => button.text() === '确认移出')

describe('NodeRemovalModal', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); get.mockResolvedValue(plan); post.mockResolvedValue({ id: 1 }) })

  it('loads selected scope, requires full node name and confirms temporary data explicitly', async () => {
    get.mockResolvedValue({ ...plan, drain: { ...plan.drain, requires_empty_dir_confirmation: [{ namespace: 'default', name: 'cache', reason: 'emptyDir' }] } })
    const wrapper = render()
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    await flushPromises()
    expect(get).toHaveBeenCalledWith('/clusters/9/nodes/4/removal-plan', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('default/web')
    await wrapper.get('#removal-node-name').setValue('worker')
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(confirm(wrapper).attributes('disabled')).toBeUndefined()
    await confirm(wrapper).trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/nodes/4/remove', { confirm_node_name: 'worker', node_uid: 'uid-1', delete_empty_dir_data: true })
    expect(wrapper.emitted('started')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('blocks submission when unmanaged Pods need attention', async () => {
    get.mockResolvedValue({ ...plan, can_start: false, drain: { ...plan.drain, blocked: [{ namespace: 'default', name: 'debug', reason: '未受控制器管理' }] } })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#removal-node-name').setValue('worker')
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('default/debug')
    expect(post).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('uses the shared error modal and retains the form after an API failure', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#removal-node-name').setValue('worker')
    post.mockRejectedValue(new Error('节点身份已变化，请重新检查'))
    await confirm(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ErrorNoticeModal).props('message')).toContain('节点身份已变化')
    expect(wrapper.emitted('started')).toBeUndefined()
    expect(wrapper.get('#removal-node-name').element.value).toBe('worker')
    wrapper.unmount()
  })

  it('discards stale preflight responses after switching cluster and node', async () => {
    let resolveOld
    get.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    await wrapper.setProps({ clusterId: '10', server: { id: 5 } })
    await flushPromises()
    resolveOld({ ...plan, node_name: 'stale-node' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-node')
    expect(get).toHaveBeenLastCalledWith('/clusters/10/nodes/5/removal-plan', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    wrapper.unmount()
  })

  it('keeps submission disabled after a failed preflight and supports rechecking', async () => {
    get.mockRejectedValueOnce(new Error('Agent 无法连接'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.findComponent(ErrorNoticeModal).props('message')).toContain('Agent 无法连接')
    expect(confirm(wrapper).attributes('disabled')).toBeDefined()
    wrapper.findComponent(ErrorNoticeModal).vm.$emit('close')
    await wrapper.findAll('button').find(button => button.text() === '重新检查').trigger('click')
    await flushPromises()
    expect(wrapper.find('#removal-node-name').exists()).toBe(true)
    wrapper.unmount()
  })
})
