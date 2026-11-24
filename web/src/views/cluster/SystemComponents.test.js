import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SystemComponents from './SystemComponents.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: apiMocks }))

const item = (overrides = {}) => ({
  chart_name: 'coredns',
  namespace: 'kube-system',
  values_content: '',
  has_config: false,
  apply_status: '',
  apply_error: '',
  controller_mode: 'static_deployment',
  detection_evidence: ['发现同名 Deployment kube-system/coredns'],
  capabilities: { configure: true, node_placement: true, rollout: true, replica_scaling: true, safe_baseline: true, restore: true },
  availability: { default_replicas: 1, high_availability: true, node_placement: true, description: '支持高可用；启用前需要至少两个可调度节点。' },
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
    expect(wrapper.get('.system-component-table .badge-online').attributes('title')).toContain('自动调度')
    expect(wrapper.text()).not.toContain('发现同名 Deployment')
    expect(wrapper.text()).not.toContain('rancher/mirrored-coredns')
  })

  it('presents a loading failure in the shared error dialog', async () => {
    apiMocks.get.mockRejectedValueOnce(new Error('系统组件读取失败'))
    const wrapper = mount(SystemComponents)
    await flushPromises()

    expect(wrapper.get('[role="dialog"]').text()).toContain('系统组件读取失败')
    expect(wrapper.find('.k8s-banner').exists()).toBe(false)
  })

  it('applies safe rollout baseline for coredns and saves', async () => {
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    await wrapper.findAll('.modal-actions button').find(button => button.text() === '高可用滚动基线').trigger('click')
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
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
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

  it('marks embedded servicelb as running and disables configuration', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'servicelb', deployment: null, deployment_error: 'deployments.apps "servicelb" not found', lb_active: true, controller_mode: 'embedded', capabilities: { configure: false, node_placement: false, rollout: false, restore: false } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.get('.mode-embedded').attributes('title')).toContain('由 K3s 内置控制器提供')
    expect(wrapper.findAll('button').find(button => button.text() === '配置').attributes('disabled')).toBeDefined()
  })

  it('shows a warning when saved values did not take effect', async () => {
    apiMocks.get.mockResolvedValue([
      item({ has_config: true, apply_status: 'succeeded', effective: false, effective_detail: 'maxUnavailable、maxSurge 与实际 Deployment 不一致' }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('已保存未生效')
    expect(wrapper.get('.badge-danger').attributes('title')).toContain('配置与实际状态不一致，请重新保存')
  })

  it('does not offer a generic double-replica baseline for an unsupported static component', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'metrics-server', controller_mode: 'static_deployment', capabilities: { configure: true, node_placement: false, rollout: true, restore: true, replica_scaling: false, safe_baseline: false }, availability: { default_replicas: 1, high_availability: false, node_placement: false, description: '副本由 K3s 管理，平台不提供通用双副本基线。' } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('K3s 静态组件')
    expect(wrapper.get('.badge-offline').attributes('title')).toContain('副本由 K3s 管理')
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    expect(wrapper.find('[data-testid="coredns-node-selector"]').exists()).toBe(false)
    expect(wrapper.findAll('.modal-actions button').some(button => button.text() === '高可用滚动基线')).toBe(false)
  })

  it('keeps an unknown control source read-only', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'metrics-server', deployment: null, controller_mode: 'unknown', capabilities: { configure: false, node_placement: false, rollout: false, restore: false } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('待确认')
    expect(wrapper.text()).toContain('控制源未识别')
    expect(wrapper.findAll('button').find(button => button.text() === '配置').attributes('disabled')).toBeDefined()
  })

  it('does not call a HelmChart component uninstalled when its chart exists without an exact Deployment name', async () => {
    apiMocks.get.mockResolvedValue([
      item({ chart_name: 'traefik', controller_mode: 'helm_chart', deployment: null, workload: null, chart_ready: true, capabilities: { configure: true, node_placement: false, rollout: false, restore: true } }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.text()).toContain('已安装')
    expect(wrapper.text()).not.toContain('未安装')
  })

  it('configures the Traefik read timeout through a dedicated control', async () => {
    apiMocks.get.mockResolvedValue([
      item({
        chart_name: 'traefik',
        controller_mode: 'helm_chart',
        values_content: 'maxUnavailable: 0\nmaxSurge: 1\n',
        capabilities: { configure: true, node_placement: false, rollout: false, restore: true },
        traefik: { read_timeout: '', effective_read_timeout: '60s', read_timeout_effective: true },
      }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    await wrapper.get('[data-testid="traefik-read-timeout"]').setValue('30m')
    await wrapper.get('form').trigger('submit')
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/traefik', {
      values_content: expect.stringContaining('maxUnavailable: 0'),
      traefik_read_timeout: '30m',
    })
  })

  it('keeps the component modal and form values when saving fails', async () => {
    apiMocks.put.mockRejectedValueOnce(new Error('组件配置写入失败'))
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    await wrapper.get('input[type="number"]').setValue('3')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.get('input[type="number"]').element.value).toBe('3')
    expect(wrapper.text()).toContain('组件配置写入失败')
  })

  it('accepts a validated custom Traefik read timeout', async () => {
    apiMocks.get.mockResolvedValue([
      item({
        chart_name: 'traefik',
        controller_mode: 'helm_chart',
        capabilities: { configure: true, node_placement: false, rollout: false, restore: true },
        traefik: { read_timeout: '', effective_read_timeout: '60s', read_timeout_effective: true },
      }),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    await wrapper.get('[data-testid="traefik-read-timeout"]').setValue('custom')
    await wrapper.get('[data-testid="traefik-custom-read-timeout"]').setValue('15m')
    await wrapper.get('form').trigger('submit')
    expect(apiMocks.put).toHaveBeenCalledWith('/system-components/traefik', expect.objectContaining({
      traefik_read_timeout: '15m',
    }))
  })

  it('shows a pending Traefik timeout without offering it for other charts', async () => {
    apiMocks.get.mockResolvedValue([
      item({
        chart_name: 'traefik',
        controller_mode: 'helm_chart',
        has_config: true,
        capabilities: { configure: true, node_placement: false, rollout: false, restore: true },
        traefik: { read_timeout: '30m', effective_read_timeout: '', read_timeout_effective: false },
      }),
      item(),
    ])
    const wrapper = mount(SystemComponents)
    await flushPromises()
    expect(wrapper.get('.system-component-table td:nth-child(5) .badge').attributes('title')).toContain('读取超时 30m，等待生效')
    await wrapper.findAll('button').find(button => button.text() === '配置').trigger('click')
    expect(wrapper.find('[data-testid="traefik-read-timeout"]').exists()).toBe(true)
  })
})
