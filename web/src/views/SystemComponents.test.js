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
  controller_mode: 'static_deployment',
  detection_evidence: ['发现同名 Deployment kube-system/coredns'],
  capabilities: { configure: true, node_placement: true, rollout: true, restore: true },
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
    apiMocks.get.mockResolvedValue([item(), item({ chart_name: 'traefik', controller_mode: 'helm_chart', capabilities: { configure: true, node_placement: false, rollout: false, restore: true } })])
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
    expect(wrapper.get('input[type="number"]').element.value).toBe('2')
    expect(wrapper.findAll('.form-select')[0].element.value).toBe('0')
    expect(wrapper.findAll('.form-select')[1].element.value).toBe('1')
    await wrapper.get('form').trigger('submit')
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/coredns', { values_content: expect.stringContaining('deploymentStrategy:') })
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/coredns', { values_content: expect.stringContaining('maxUnavailable: 0') })
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/coredns', { values_content: expect.stringContaining('replicas: 2') })
  })

  it('pins CoreDNS to a selected node when saving its configuration', async () => {
    apiMocks.get.mockImplementation(url => {
      if (url === '/nodes') return Promise.resolve([{ name: 'worker-b', ready: true, evicted: false }])
      return Promise.resolve([item()])
    })
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑配置').trigger('click')
    await wrapper.get('[data-testid="coredns-node-selector"]').setValue('worker-b')
    await wrapper.get('form').trigger('submit')

    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/coredns', {
      values_content: expect.stringContaining('kubernetes.io/hostname: worker-b'),
    })
  })

  it('reverts a configured component after confirmation', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMocks.get.mockResolvedValue([item({ has_config: true, apply_status: 'succeeded' })])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '恢复默认').trigger('click')
    expect(apiMocks.post).toHaveBeenCalledWith('/system-components/coredns/revert')
  })

  it('marks embedded servicelb as running and hides configuration', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'servicelb', deployment: null, deployment_error: 'deployments.apps "servicelb" not found', lb_active: true, controller_mode: 'embedded', capabilities: { configure: false, node_placement: false, rollout: false, restore: false } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('内置运行中')
    expect(wrapper.text()).toContain('由 K3s 进程内提供')
    expect(wrapper.text()).not.toContain('编辑配置')
  })

  it('shows a warning when saved values did not take effect', async () => {
    apiMocks.get.mockResolvedValue([
      item({ has_config: true, apply_status: 'succeeded', effective: false, effective_detail: 'maxUnavailable、maxSurge 与实际 Deployment 不一致' }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('已保存未生效')
    expect(wrapper.text()).toContain('maxUnavailable、maxSurge 与实际 Deployment 不一致')
  })

  it('uses the detected static deployment capability instead of a component-name special case', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'metrics-server', controller_mode: 'static_deployment', capabilities: { configure: true, node_placement: true, rollout: true, restore: true } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('控制方式：K3s 静态 Deployment')
    expect(wrapper.text()).toContain('迁移')
    await wrapper.findAll('button').find(button => button.text() === '编辑配置').trigger('click')
    expect(wrapper.find('[data-testid="coredns-node-selector"]').exists()).toBe(true)
  })

  it('keeps an unknown control source read-only', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'metrics-server', deployment: null, controller_mode: 'unknown', capabilities: { configure: false, node_placement: false, rollout: false, restore: false } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('控制方式：未识别')
    expect(wrapper.text()).toContain('控制源未识别')
    expect(wrapper.text()).not.toContain('编辑配置')
  })
})
