import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SystemComponents from './SystemComponents.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('../api/index.js', () => ({ api: apiMocks }))

const item = (overrides = {}) => ({
  chart_name: 'coredns',
  namespace: 'kube-system',
  values_content: '',
  has_config: false,
  apply_status: '',
  apply_error: '',
  deployment: {
    replicas: 1,
    ready_replicas: 1,
    strategy: { type: 'RollingUpdate', rollingUpdate: { maxUnavailable: '1', maxSurge: '25%' } },
    image: 'rancher/mirrored-coredns-coredns:1.14.4',
  },
  ...overrides,
})

describe('SystemComponents', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.get.mockResolvedValue([item(), item({ chart_name: 'traefik' })])
    apiMocks.put.mockResolvedValue({})
    apiMocks.post.mockResolvedValue({})
  })

  it('renders components with deployment state', async () => {
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('coredns')
    expect(wrapper.text()).toContain('traefik')
    expect(wrapper.text()).toContain('1/1 就绪')
    expect(wrapper.text()).toContain('RollingUpdate U:1 S:25%')
  })

  it('applies safe rollout baseline for coredns and saves', async () => {
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑配置').trigger('click')
    await wrapper.findAll('.modal-actions button').find(button => button.text() === '安全滚动基线').trigger('click')
    expect(wrapper.get('textarea').element.value).toContain('replicas: 2')
    await wrapper.get('form').trigger('submit')
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/coredns', { values_content: expect.stringContaining('maxUnavailable: 0') })
  })

  it('reverts a configured component after confirmation', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMocks.get.mockResolvedValue([item({ has_config: true, apply_status: 'succeeded' })])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '恢复默认').trigger('click')
    expect(apiMocks.post).toHaveBeenCalledWith('/system-components/coredns/revert')
  })
})
