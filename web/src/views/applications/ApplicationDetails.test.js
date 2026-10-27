import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ApplicationDetails from './ApplicationDetails.vue'

const routerPush = vi.hoisted(() => vi.fn())
vi.mock('vue-router', () => ({ useRouter: () => ({ push: routerPush }), useRoute: () => ({ query: {} }) }))

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/applications/1') return Promise.resolve({ application: { id: 1, project_id: 2, environment_id: 3, name: 'order-api', workload_kind: 'deployment', capabilities: ['hysteria2'], project: { name: 'commerce' }, environment: { name: 'production', cluster_id: 2, namespace: 'commerce-prod' } }, releases: [{ id: 3, sequence: 2, image: 'registry.example.com/order-api:2.0.0', status: 'succeeded' }] })
      if (path === '/applications/1/deployment-templates') return Promise.resolve([{ id: 4, name: '标准生产配置', enabled: true, is_default: true, revision: 2, spec: { image: 'registry.example.com/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      if (path === '/applications/1/endpoints') return Promise.resolve([{ id: 7, domain_id: 4, domain: 'api.example.com', path: '/', service_port: 80, tls_enabled: true, access_mode: 'protected_console' }, { id: 8, domain_id: 5, domain: 'admin.example.com', path: '/console', service_port: 80, tls_enabled: false, access_mode: 'public' }])
      if (path === '/domains?environment_id=3') return Promise.resolve([{ id: 4, hostname: 'api.example.com', enabled: true, certificate: { status: 'Ready' } }, { id: 5, hostname: 'admin.example.com', enabled: true, certificate: { status: 'Ready' } }])
      if (path === '/clusters/2/k8s/configmaps?namespace=commerce-prod&usage=false') return Promise.resolve([])
      if (path === '/k8s/configmaps/commerce-prod/app-config') return Promise.resolve({ data: { 'config.yaml': 'port: 8080' } })
      if (path === '/clusters/2/k8s/secrets?namespace=commerce-prod&usage=false') return Promise.resolve([{ name: 'edge-tls', keys: ['tls.crt', 'tls.key'], type: 'kubernetes.io/tls' }])
      return Promise.resolve([])
    }),
    post: vi.fn(), put: vi.fn(), delete: vi.fn(),
  },
}))

describe('ApplicationDetails view', () => {
  it('opens domain management for the current application environment', async () => {
    routerPush.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '访问入口').trigger('click')
    await wrapper.find('.detail-access-card .detail-heading-actions .btn:not(.btn-primary)').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ path: '/applications/domains', query: {
      project_id: 2, environment_id: 3, application_id: '1', return_tab: 'access',
    } })

    await wrapper.find('.detail-access-card .btn-primary').trigger('click')
    await wrapper.find('.domain-management-link').trigger('click')
    expect(routerPush).toHaveBeenCalledTimes(2)
  })

  it('shows release history on a dedicated application drill-down page', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.back-link').text()).toContain('返回工作台')
    expect(wrapper.find('.page-title').text()).toBe('order-api')
    expect(wrapper.text()).toContain('Release #2')
    expect(wrapper.text()).toContain('registry.example.com/order-api:2.0.0')
    expect(wrapper.text()).toContain('标准生产配置')
    expect(wrapper.text()).toContain('api.example.com')
    expect(wrapper.text()).toContain('admin.example.com')
    expect(wrapper.find('.application-settings-section').exists()).toBe(false)
    expect(wrapper.find('.section-tabs-actions [aria-label="应用设置"]').exists()).toBe(true)
    expect(wrapper.find('.capability-list').exists()).toBe(false)
    expect(wrapper.find('.detail-config-card > :first-child').classes()).toContain('template-section')
    expect(wrapper.find('.deploy-webhook-details').attributes('open')).toBeUndefined()
    await wrapper.find('[aria-label="应用设置"]').trigger('click')
    expect(wrapper.find('[aria-label="工作负载类型"].select-menu-native').element.value).toBe('deployment')
  })

  it('manages a per-application deploy webhook from deployment configuration', async () => {
    const { api } = await import('../../api/index.js')
    const originalGet = api.get.getMockImplementation()
    api.get.mockImplementation((path, options) => path === '/applications/1/deploy-webhook'
      ? Promise.resolve({ configured: false }) : originalGet(path, options))
    api.post.mockReset()
    api.post.mockResolvedValue({ secret: 'generated-secret' })
    api.delete.mockReset()
    api.delete.mockResolvedValue({ configured: false })
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(wrapper.find('.deploy-webhook-section .badge').text()).toBe('未启用')
      expect(wrapper.find('[aria-label="自动发布回调地址"]').element.value).toContain('/api/applications/1/deployments')

      await wrapper.find('.deploy-webhook-section .btn').trigger('click')
      await wrapper.find('.base-modal-actions .btn-primary').trigger('click')
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(api.post).toHaveBeenCalledWith('/applications/1/deploy-webhook/secret')
      expect(wrapper.text()).toContain('generated-secret')
      expect(wrapper.find('.deploy-webhook-section .badge').text()).toBe('已启用')

      await wrapper.find('.base-modal-actions .btn-primary').trigger('click')
      await wrapper.find('.deploy-webhook-section .btn-danger').trigger('click')
      await wrapper.find('.base-modal-actions .btn-danger').trigger('click')
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(api.delete).toHaveBeenCalledWith('/applications/1/deploy-webhook')
      expect(wrapper.find('.deploy-webhook-section .badge').text()).toBe('未启用')
    } finally {
      wrapper.unmount()
      api.get.mockImplementation(originalGet)
    }
  })

  it('allows retrying when deploy webhook status cannot be loaded', async () => {
    const { api } = await import('../../api/index.js')
    const originalGet = api.get.getMockImplementation()
    api.get.mockImplementation((path, options) => path === '/applications/1/deploy-webhook'
      ? Promise.reject(new Error('API 路径不存在')) : originalGet(path, options))
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(wrapper.find('.deploy-webhook-section .badge').text()).toBe('读取失败')
      expect(wrapper.find('.deploy-webhook-section [aria-label="重试读取"]').exists()).toBe(true)
      expect(wrapper.find('.deploy-webhook-section .btn').exists()).toBe(false)
    } finally {
      wrapper.unmount()
      api.get.mockImplementation(originalGet)
    }
  })

  it('refreshes release status while the history tab is open and stops after leaving it', async () => {
    const { api } = await import('../../api/index.js')
    const originalGet = api.get.getMockImplementation()
    let status = 'draft'
    api.get.mockImplementation((path, options) => path === '/applications/1'
      ? Promise.resolve({ application: { id: 1, name: 'order-api' }, releases: [{ id: 3, sequence: 2, status }] })
      : originalGet(path, options))
    vi.useFakeTimers()
    let wrapper
    try {
      wrapper = mount(ApplicationDetails, {
        props: { applicationID: '1' },
        global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
      })
      await vi.advanceTimersByTimeAsync(0)
      await wrapper.findAll('.section-tab').find(tab => tab.text() === '发布记录').trigger('click')
      await vi.advanceTimersByTimeAsync(0)
      expect(wrapper.find('.detail-releases-card .badge').text()).toBe('draft')

      status = 'succeeded'
      await vi.advanceTimersByTimeAsync(5000)
      expect(wrapper.find('.detail-releases-card .badge').text()).toBe('succeeded')

      await wrapper.findAll('.section-tab').find(tab => tab.text() === '运行概览').trigger('click')
      const requestCount = api.get.mock.calls.filter(([path]) => path === '/applications/1').length
      await vi.advanceTimersByTimeAsync(5000)
      expect(api.get.mock.calls.filter(([path]) => path === '/applications/1')).toHaveLength(requestCount)
    } finally {
      wrapper?.unmount()
      vi.useRealTimers()
      api.get.mockImplementation(originalGet)
    }
  })

  it('restarts the application from its latest successful release', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.post.mockResolvedValue({ id: 9, sequence: 3, status: 'pending' })
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('.runtime-toolbar .btn').trigger('click')
    await nextTick()
    expect(wrapper.find('.restart-modal').exists()).toBe(true)
    expect(api.post).not.toHaveBeenCalled()
    await wrapper.find('.restart-modal .btn-primary').trigger('click')
    await nextTick()
    expect(api.post).toHaveBeenCalledWith('/applications/1/restarts')
  })

  it('edits generic application capability labels without modifying templates', async () => {
    const { api } = await import('../../api/index.js')
    api.put.mockReset()
    api.put.mockResolvedValue({ id: 1, name: 'order-api', capabilities: ['hysteria2', 'metrics'] })
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('[aria-label="应用设置"]').trigger('click')
    expect(wrapper.find('.capability-list input').element.value).toBe('hysteria2')
    await wrapper.find('.capability-section .section-heading .btn').trigger('click')
    await wrapper.findAll('[aria-label="能力标签"]')[1].setValue('metrics')
    await wrapper.find('.capability-section .base-modal-actions .btn-primary').trigger('click')

    expect(api.put).toHaveBeenCalledWith('/applications/1/capabilities', { capabilities: ['hysteria2', 'metrics'] })
    expect(api.put).not.toHaveBeenCalledWith(expect.stringContaining('deployment-templates'), expect.anything())
    expect(wrapper.find('.capability-section').exists()).toBe(false)
    await wrapper.find('[aria-label="应用设置"]').trigger('click')
    expect(wrapper.findAll('[aria-label="能力标签"]').map(input => input.element.value)).toEqual(['hysteria2', 'metrics'])
    wrapper.unmount()
  })

  it('allows an empty capability list and shows capability API errors', async () => {
    const { api } = await import('../../api/index.js')
    api.put.mockReset()
    api.put.mockRejectedValue(new Error('能力标签格式无效'))
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('[aria-label="应用设置"]').trigger('click')
    await wrapper.find('.capability-row .icon-button').trigger('click')
    await wrapper.find('.capability-section .base-modal-actions .btn-primary').trigger('click')

    expect(api.put).toHaveBeenCalledWith('/applications/1/capabilities', { capabilities: [] })
    expect(document.body.querySelector('.error-notice')?.textContent).toContain('能力标签格式无效')
    wrapper.unmount()
  })

  it('opens a bound application endpoint through a handoff', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.post.mockResolvedValue({ handoff_url: 'https://api.example.com/?handoff_code=one-time-code' })
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.endpoint-url')[0].trigger('click')
    expect(api.post).toHaveBeenCalledWith('/applications/1/integration-handoffs', { endpoint_id: 7 })
    expect(open).toHaveBeenCalledWith('https://api.example.com/?handoff_code=one-time-code', '_blank', 'noopener,noreferrer')
    open.mockRestore()
  })

  it('opens a public endpoint directly without requesting a handoff', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.endpoint-url')[1].trigger('click')
    expect(api.post).not.toHaveBeenCalled()
    expect(open).toHaveBeenCalledWith('http://admin.example.com/console', '_blank', 'noopener,noreferrer')
    open.mockRestore()
  })

  it('uses a browser-local address when opening a protected console for local debugging', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.post.mockResolvedValue({ handoff_url: 'http://localhost:5178/?handoff_code=one-time-code' })
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.find('.endpoint-row .btn').trigger('click')
    await wrapper.find('input[placeholder="http://localhost:5178"]').setValue('http://localhost:5178/console')
    await wrapper.find('form').trigger('submit.prevent')
    expect(api.post).toHaveBeenCalledWith('/applications/1/integration-handoffs', { endpoint_id: 7, redirect_url: 'http://localhost:5178/console' })
    expect(window.localStorage.getItem('cylism.protected-console.local-url:7')).toBe('http://localhost:5178/console')
    open.mockRestore()
  })

  it('serializes row-based startup and environment values into the template spec', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="1.4.2"]').setValue('2.3.4')
    await wrapper.findAll('button').find(button => button.text().includes('添加命令')).trigger('click')
    const addArgument = wrapper.findAll('button').find(button => button.text().includes('添加参数'))
    await addArgument.trigger('click')
    await addArgument.trigger('click')
    const valueLists = wrapper.findAll('.value-list')
    await valueLists[0].find('input').setValue('/usr/bin/chromium-browser')
    await valueLists[1].findAll('input')[0].setValue('--no-sandbox')
    await valueLists[1].findAll('input')[1].setValue('--remote-debugging-port=9222')
    const addVariables = wrapper.findAll('button').filter(button => button.text().includes('添加变量'))
    await addVariables[0].trigger('click')
    await addVariables[1].trigger('click')
    const keyValueLists = wrapper.findAll('.key-value-list')
    await keyValueLists[0].findAll('input')[0].setValue('LOG_LEVEL')
    await keyValueLists[0].find('textarea').setValue('debug')
    await keyValueLists[1].findAll('input')[0].setValue('DATABASE_PASSWORD')
    await keyValueLists[1].find('textarea').setValue('secret-value')
    await wrapper.find('form').trigger('submit.prevent')

    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      default_version: '2.3.4',
      spec: expect.objectContaining({
        command: ['/usr/bin/chromium-browser'],
        args: ['--no-sandbox', '--remote-debugging-port=9222'],
        config: { LOG_LEVEL: 'debug' },
        secrets: { DATABASE_PASSWORD: 'secret-value' },
      }),
    }))
  })

  it('groups template fields into deployment sections with a fixed action bar', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    expect(wrapper.findAll('.editor-section-heading h3').map(heading => heading.text())).toEqual(['基础信息', '服务部署', '配置与存储', '资源与健康', '服务网络'])
    expect(wrapper.find('.editor-actions .btn-primary').text()).toBe('保存模板')
    expect(wrapper.find('.editor-actions .btn-primary').attributes('form')).toBe('template-editor-form')
    expect(wrapper.text()).toContain('仅集群内访问；HTTP 服务可通过域名入口暴露。')
    const serviceType = wrapper.findAll('.editor-modal select').find(select => select.findAll('option').some(option => option.element.value === 'LoadBalancer'))
    await serviceType.setValue('LoadBalancer')
    expect(wrapper.text()).toContain('由集群负载均衡器分配外部地址，适用于 TCP 或 UDP 直出。')
  })

  it('shows one template section at a time and preserves edits across sections', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.post.mockResolvedValue({})
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.find('.detail-config-card .btn-primary').trigger('click')
      const visibleSections = () => wrapper.findAll('.editor-section').filter(section => section.isVisible()).map(section => section.attributes('id'))
      const selectSection = label => wrapper.findAll('.editor-nav button').find(button => button.text().includes(label)).trigger('click')
      expect(visibleSections()).toEqual(['template-basic'])
      await wrapper.find('[placeholder="标准生产配置"]').setValue('test-template')
      await selectSection('配置与存储')
      expect(visibleSections()).toEqual(['template-storage'])
      expect(wrapper.findAll('#template-storage .editor-disclosure')).toHaveLength(5)
      expect(wrapper.findAll('#template-storage .editor-disclosure').every(item => item.attributes('open') === undefined)).toBe(true)
      await wrapper.findAll('#template-storage button').find(button => button.text() === '+ 添加变量').trigger('click')
      await wrapper.find('#template-storage [placeholder="变量名或文件名"]').setValue('LOG_LEVEL')
      await wrapper.find('#template-storage textarea').setValue('debug')
      await selectSection('服务网络')
      expect(visibleSections()).toEqual(['template-network'])
      await wrapper.find('[aria-label="DNS 解析策略"].select-menu-native').setValue('1')
      await selectSection('基础信息')
      expect(wrapper.find('[placeholder="标准生产配置"]').element.value).toBe('test-template')
      await wrapper.find('#template-editor-form').trigger('submit.prevent')
      expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({ name: 'test-template', spec: expect.objectContaining({ dns_ndots: 1, config: { LOG_LEVEL: 'debug' } }) }))
    } finally { wrapper.unmount() }
  })

  it('reveals the section and disclosure containing an invalid required field', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' }, attachTo: document.body,
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.find('.detail-config-card .btn-primary').trigger('click')
      const storage = wrapper.findAll('.editor-nav button').find(button => button.text().includes('配置与存储'))
      await storage.trigger('click')
      const mounts = wrapper.findAll('#template-storage .editor-disclosure').find(item => item.find('summary').text().includes('配置文件挂载'))
      await mounts.find('.btn').trigger('click')
      const field = wrapper.find('[placeholder="/etc/app/config.yaml"]')
      const currentMounts = wrapper.findAll('#template-storage .editor-disclosure').find(item => item.find('summary').text().includes('配置文件挂载'))
      currentMounts.element.open = false
      await wrapper.findAll('.editor-nav button').find(button => button.text().includes('基础信息')).trigger('click')
      expect(wrapper.find('#template-storage').isVisible()).toBe(false)
      await field.trigger('invalid')
      await nextTick()
      expect(wrapper.find('#template-storage').isVisible()).toBe(true)
      expect(wrapper.find('[placeholder="/etc/app/config.yaml"]').element.closest('details').open).toBe(true)
      expect(wrapper.find('#template-storage').isVisible()).toBe(true)
    } finally { wrapper.unmount() }
  })

  it('keeps an automatic NodePort empty and explains HTTP Ingress setup for TCP ports', async () => {
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    const serviceType = wrapper.findAll('.editor-modal select').find(select => select.findAll('option').some(option => option.element.value === 'NodePort'))
    await serviceType.setValue('NodePort')

    expect(wrapper.find('[aria-label="NodePort"]').element.value).toBe('')
    expect(wrapper.text()).toContain('留空由 Kubernetes 自动分配')
    expect(wrapper.text()).toContain('保存模板并发布后，在页面顶部“对外域名”绑定已就绪域名')
    expect(wrapper.text()).toContain('Ingress 只转发 HTTP/HTTPS')
  })

  it('serializes multi-protocol Service ports and Secret file projection settings', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    const serviceType = wrapper.findAll('.editor-modal select').find(select => select.findAll('option').some(option => option.element.value === 'LoadBalancer'))
    await serviceType.setValue('LoadBalancer')
    const addPortButton = wrapper.findAll('button').find(button => button.text().includes('添加端口'))
    await addPortButton.trigger('click')
    const servicePorts = wrapper.findAll('.service-port-row')
    await servicePorts[0].find('[aria-label="Service 端口名称"]').setValue('proxy')
    wrapper.vm.templateForm.service.ports[0].protocol = 'UDP'
    await servicePorts[0].find('[aria-label="Service 端口"]').setValue(443)
    await servicePorts[0].find('[aria-label="Target Port"]').setValue(443)
    await servicePorts[1].find('[aria-label="Service 端口名称"]').setValue('api')
    await servicePorts[1].find('[aria-label="Service 端口"]').setValue(8080)
    const addButtons = wrapper.findAll('button').filter(button => button.text().includes('添加挂载'))
    const addButton = addButtons[addButtons.length - 1]
    await addButton.trigger('click')
    const fileRow = wrapper.find('.file-mount-row')
    const fileSelects = fileRow.findAll('select')
    await fileSelects[0].setValue('secret')
    await nextTick()
    wrapper.vm.templateForm.file_mounts[0].source_name = 'edge-tls'
    wrapper.vm.templateForm.file_mounts[0].key = 'tls.crt'
    await nextTick()
    await wrapper.find('.file-mount-row [placeholder="/etc/app/config.yaml"]').setValue('/run/app/tls/tls.crt')
    await wrapper.find('form').trigger('submit.prevent')

    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      spec: expect.objectContaining({
        health: expect.objectContaining({ readiness_enabled: true, liveness_enabled: false }),
        service: expect.objectContaining({ type: 'LoadBalancer', ports: [
          { name: 'proxy', port: 443, target_port: 443, protocol: 'UDP', node_port: 0 },
          { name: 'api', port: 8080, target_port: 8080, protocol: 'TCP', node_port: 0 },
        ] }),
        file_mounts: [{ source_type: 'secret', source_name: 'edge-tls', key: 'tls.crt', mount_path: '/run/app/tls/tls.crt' }],
      }),
    }))
  })

  it('discards canceled capability edits and migrates workloads only on confirmation', async () => {
    const { api } = await import('../../api/index.js')
    api.put.mockReset()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.find('[aria-label="应用设置"]').trigger('click')
      await wrapper.find('[aria-label="能力标签"]').setValue('discard-me')
      await wrapper.find('.capability-section .base-modal-actions .btn').trigger('click')
      expect(api.put).not.toHaveBeenCalled()
      await wrapper.find('[aria-label="应用设置"]').trigger('click')
      expect(wrapper.find('[aria-label="能力标签"]').element.value).toBe('hysteria2')
      await wrapper.find('.capability-section .base-modal-actions .btn').trigger('click')
      await wrapper.find('[aria-label="应用设置"]').trigger('click')
      expect(wrapper.find('[aria-label="修改工作负载类型"]').element.disabled).toBe(true)
      await wrapper.find('[aria-label="工作负载类型"].select-menu-native').setValue('statefulset')
      expect(api.put).not.toHaveBeenCalled()
      await wrapper.find('[aria-label="修改工作负载类型"]').trigger('click')
      expect(wrapper.find('.application-settings-modal').exists()).toBe(false)
      expect(wrapper.find('.workload-migration-summary').text()).toBe('Deployment → StatefulSet')
      expect(api.put).not.toHaveBeenCalled()
      await wrapper.find('.workload-editor .base-modal-actions .btn').trigger('click')
      expect(wrapper.find('.application-settings-modal').exists()).toBe(true)
      expect(wrapper.find('[aria-label="工作负载类型"].select-menu-native').element.value).toBe('statefulset')
      await wrapper.find('[aria-label="修改工作负载类型"]').trigger('click')
      api.put.mockResolvedValue({ id: 1, name: 'order-api', workload_kind: 'statefulset' })
      await wrapper.find('.workload-editor .btn-primary').trigger('click')
      await nextTick()
      expect(api.put).toHaveBeenCalledWith('/applications/1/workload-kind', { workload_kind: 'statefulset' })
      expect(wrapper.find('.workload-editor').exists()).toBe(false)
      expect(wrapper.find('.application-settings-modal').exists()).toBe(true)
      expect(wrapper.find('[aria-label="工作负载类型"].select-menu-native').element.value).toBe('statefulset')
      expect(wrapper.find('[aria-label="修改工作负载类型"]').element.disabled).toBe(true)
    } finally { wrapper.unmount() }
  })

  it.each([['', undefined], ['1', 1]])('saves the selected DNS strategy %s on a new template', async (choice, expected) => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.post.mockResolvedValue({})
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      await wrapper.find('.detail-config-card .btn-primary').trigger('click')
      const strategy = wrapper.find('[aria-label="DNS 解析策略"].select-menu-native')
      expect(strategy.element.value).toBe('')
      await strategy.setValue(choice)
      await wrapper.find('#template-editor-form').trigger('submit.prevent')
      const payload = api.post.mock.calls.find(([path]) => path === '/applications/1/deployment-templates')[1]
      expect(payload.spec.dns_ndots).toBe(expected)
      expect(Object.hasOwn(payload.spec, 'dns_ndots')).toBe(expected !== undefined)
    } finally { wrapper.unmount() }
  })

  it.each([0, 1, 5])('preserves existing ndots=%s and allows returning to cluster defaults', async value => {
    const { api } = await import('../../api/index.js')
    const originalGet = api.get.getMockImplementation()
    api.get.mockImplementation(async (path, options) => {
      const result = await originalGet(path, options)
      return path === '/applications/1/deployment-templates' ? result.map(item => ({ ...item, spec: { ...item.spec, dns_ndots: value } })) : result
    })
    api.put.mockReset()
    api.put.mockResolvedValue({})
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    try {
      await new Promise(resolve => setTimeout(resolve, 0))
      const edit = () => wrapper.findAll('.template-section .btn').find(button => button.text() === '编辑').trigger('click')
      await edit()
      expect(wrapper.find('[aria-label="DNS 解析策略"].select-menu-native').element.value).toBe(String(value))
      await wrapper.find('#template-editor-form').trigger('submit.prevent')
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(api.put).toHaveBeenLastCalledWith('/applications/1/deployment-templates/4', expect.objectContaining({ spec: expect.objectContaining({ dns_ndots: value }) }))
      await edit()
      await wrapper.find('[aria-label="DNS 解析策略"].select-menu-native').setValue('')
      await wrapper.find('#template-editor-form').trigger('submit.prevent')
      const payload = api.put.mock.calls.at(-1)[1]
      expect(Object.hasOwn(payload.spec, 'dns_ndots')).toBe(false)
    } finally { wrapper.unmount(); api.get.mockImplementation(originalGet) }
  })

  it('serializes host networking from the deployment template', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    await wrapper.find('[aria-label="使用宿主机网络"]').setValue(true)
    await wrapper.find('form').trigger('submit.prevent')

    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      spec: expect.objectContaining({ host_network: true }),
    }))
  })

  it('mounts a template ConfigMap key as a file without creating a separate resource', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockClear()
    api.post.mockResolvedValue({})
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    await wrapper.findAll('button').find(button => button.text().includes('添加变量')).trigger('click')
    const configItems = wrapper.findAll('.key-value-list')[0]
    await configItems.find('input').setValue('config.yaml')
    await configItems.find('textarea').setValue('listen: :8080\n')
    const addMountButtons = wrapper.findAll('button').filter(button => button.text().includes('添加挂载'))
    await addMountButtons[addMountButtons.length - 1].trigger('click')
    await nextTick()
    const fileRow = wrapper.find('.file-mount-row')
    const fileSelects = fileRow.findAll('select')
    expect(fileSelects[0].element.value).toBe('application_config')
    expect(fileRow.find('[aria-label="文件来源资源"]').element.value).toBe('order-api-config')
    await fileSelects[1].setValue('config.yaml')
    await fileRow.find('[placeholder="/etc/app/config.yaml"]').setValue('/etc/app/config.yaml')
    await nextTick()
    await wrapper.find('form').trigger('submit.prevent')
    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      spec: expect.objectContaining({
        config: { 'config.yaml': 'listen: :8080\n' },
        file_mounts: [{ source_type: 'application_config', source_name: '', key: 'config.yaml', mount_path: '/etc/app/config.yaml' }],
      }),
    }))
    expect(api.post).not.toHaveBeenCalledWith('/k8s/configmaps', expect.anything())
  })

  it('marks a current application Secret key for external management', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockClear()
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '部署配置').trigger('click')
    await wrapper.find('.detail-config-card .btn-primary').trigger('click')
    wrapper.vm.templateForm.secret_items.push({ key: 'config.yaml', value: 'listen: :8443\n', enabled: true, externally_managed: true })
    await nextTick()
    const addMountButtons = wrapper.findAll('button').filter(button => button.text().includes('添加挂载'))
    await addMountButtons[addMountButtons.length - 1].trigger('click')
    const fileRow = wrapper.find('.file-mount-row')
    await fileRow.findAll('select')[0].setValue('application_secret')
    await nextTick()
    wrapper.vm.templateForm.file_mounts[0].key = 'config.yaml'
    await fileRow.find('[placeholder="/etc/app/config.yaml"]').setValue('/etc/app/config.yaml')
    await wrapper.find('form').trigger('submit.prevent')

    expect(api.post).toHaveBeenCalledWith('/applications/1/deployment-templates', expect.objectContaining({
      spec: expect.objectContaining({
        secrets: { 'config.yaml': 'listen: :8443\n' },
        secret_managed_keys: ['config.yaml'],
        file_mounts: [{ source_type: 'application_secret', source_name: '', key: 'config.yaml', mount_path: '/etc/app/config.yaml' }],
      }),
    }))
  })

  it('creates, edits, and unbinds individual endpoints', async () => {
    const { api } = await import('../../api/index.js')
    api.post.mockReset()
    api.put.mockReset()
    api.delete.mockReset()
    api.post.mockResolvedValue({ id: 9, domain_id: 4, domain: 'api.example.com', path: '/v2', service_port: 80, tls_enabled: true, access_mode: 'public' })
    api.put.mockResolvedValue({ id: 7, domain_id: 4, domain: 'api.example.com', path: '/v3', service_port: 80, tls_enabled: false, access_mode: 'protected_console' })
    api.delete.mockResolvedValue({ id: 7 })
    const wrapper = mount(ApplicationDetails, {
      props: { applicationID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('.section-tab').find(tab => tab.text() === '访问入口').trigger('click')
    await wrapper.find('.detail-access-card .btn-primary').trigger('click')
    expect(wrapper.find('[aria-label="访问入口"].select-menu-native').element.value).toBe('record')
    await wrapper.find('.modal .form-select').setValue('4')
    await wrapper.find('.modal .form-input').setValue('/v2')
    await wrapper.find('[aria-label="访问入口"].select-menu-native').setValue('ingress')
    await wrapper.find('.modal .check-row input').setValue(true)
    await wrapper.find('.modal form').trigger('submit.prevent')
    expect(api.post).toHaveBeenCalledWith('/applications/1/endpoints', { domain_id: 4, path: '/v2', service_port: 80, protocol: 'TCP', tls_enabled: true, ingress_enabled: true, access_mode: 'public' })

    await wrapper.findAll('.endpoint-row .btn').find(button => button.text().includes('编辑')).trigger('click')
    await wrapper.find('.modal .form-input').setValue('/v3')
    await wrapper.find('.modal .check-row input').setValue(false)
    await wrapper.find('.modal form').trigger('submit.prevent')
    expect(api.put).toHaveBeenCalledWith('/applications/1/endpoints/7', { domain_id: 4, path: '/v3', service_port: 80, protocol: 'TCP', tls_enabled: false, ingress_enabled: true, access_mode: 'protected_console' })

    await wrapper.find('.endpoint-row .btn-danger').trigger('click')
    expect(api.delete).toHaveBeenCalledWith('/applications/1/endpoints/7')
  })
})
