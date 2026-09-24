import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import SystemSettings from './SystemSettings.vue'
import SystemSettingsSecurity from './SystemSettingsSecurity.vue'
import { api } from '../../api/index.js'

const route = reactive({ path: '/settings/system', query: reactive({}) })
const routerPush = vi.fn(async ({ query }) => {
  Object.assign(route.query, query || {})
  await nextTick()
})

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ push: routerPush }),
}))

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn(path => {
      if (path === '/auth/temporary-tokens') return Promise.resolve([])
      if (path === '/platform/status') return Promise.resolve({ webhook_configured: true, image_prefix: 'registry.example.com/cylism-manager', deployment: { image: 'registry.example.com/cylism-manager:latest', ready_replicas: 1 }, releases: [{ id: 3, source: 'github', status: 'succeeded', image: 'registry.example.com/cylism-manager:latest', commit_sha: 'aabbccddeeff00112233445566778899', created_at: '2026-08-01T12:00:00Z' }] })
      if (path === '/platform/endpoint') return Promise.resolve({ endpoint: { hostname: 'console.example.com', certificate_name: 'console-example-com', enabled: true }, url: 'https://console.example.com', state: 'ready', ingress_ready: true, ingress: { namespace: 'default', name: 'cylism-ingress', ingress_class: 'traefik', hostname: 'console.example.com', path: '/', service_name: 'cylism-manager', service_port: '8080', tls_secret_name: 'console-example-com-tls' }, certificate: { name: 'console-example-com', status: 'Ready', expiry_date: '2026-10-01T12:00:00Z', renewal_time: '2026-09-01T12:00:00Z' } })
      if (path === '/certs') return Promise.resolve([{ name: 'console-example-com', namespace: 'default', status: 'Ready', domains: ['console.example.com'] }])
      return Promise.resolve({ initialized: true, ip: '100.88.0.1', online: true })
    }),
    post: vi.fn().mockResolvedValue({ secret: 'generated-secret' }),
    put: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
  },
}))

beforeEach(() => {
  document.body.innerHTML = ''
  vi.useRealTimers()
  route.query.tab = undefined
  routerPush.mockClear()
  api.get.mockClear()
  api.post.mockClear()
  api.put.mockClear()
  api.delete.mockClear()
})

describe('SystemSettings view', () => {
  it('shows tabbed settings sections', async () => {
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await nextTick()

    expect(wrapper.text()).toContain('系统设置')
    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.get('.section-tabs-header').find('h1').text()).toBe('系统设置')
    expect(wrapper.find('.page-header').exists()).toBe(false)
    expect(wrapper.find('.workspace-header').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Tailscale')
    expect(wrapper.text()).not.toContain('平台自更新')
    expect(wrapper.get('.temporary-token-card').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)

    await wrapper.get('[data-testid="system-settings-tab-entry"]').trigger('click')
    await nextTick()
    expect(wrapper.find('.workspace-header').exists()).toBe(false)
    expect(wrapper.find('.platform-entry-card .card-title').exists()).toBe(false)
    expect(wrapper.find('.platform-entry-card .surface-card-actions').exists()).toBe(false)
    expect(wrapper.get('.platform-entry-table-header').text()).toContain('配置名')
    expect(wrapper.get('.platform-entry-table-header').text()).toContain('配置值')
    expect(wrapper.get('.platform-entry-table-header').text()).toContain('操作')
    expect(wrapper.get('.platform-entry-row.domain-binding-row').text()).toContain('修改')
    expect(wrapper.get('.platform-entry-row.domain-binding-row').text()).toContain('停用')
    expect(wrapper.text()).not.toContain('临时登录秘钥')
    expect(wrapper.get('.platform-entry-card').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)

    await wrapper.get('[data-testid="system-settings-tab-release"]').trigger('click')
    await nextTick()
    expect(wrapper.find('.workspace-header').exists()).toBe(false)
    expect(wrapper.findAll('.platform-release-layout > .surface-card')).toHaveLength(2)
    expect(wrapper.find('.platform-release-config-card .card-title').exists()).toBe(false)
    expect(wrapper.get('.platform-release-config-header').text()).toContain('配置名')
    expect(wrapper.get('.platform-release-config-header').text()).toContain('配置值')
    expect(wrapper.get('.platform-release-config-header').text()).toContain('操作')
    expect(wrapper.get('.platform-release-history-header').text()).toContain('发布方式')
    expect(wrapper.get('.platform-release-history-header').text()).toContain('镜像 / Tag')
    expect(wrapper.text()).toContain('registry.example.com/cylism-manager:latest')
    expect(wrapper.get('.platform-release-config-card').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders temporary tokens as a compact list and only shows the empty state when no token exists', async () => {
    api.get.mockResolvedValueOnce([
      { id: 'revoked-1', label: '供应商临时访问', status: 'revoked', created_at: '2026-09-02T09:24:26Z', expires_at: '2026-09-09T09:24:26Z' },
      { id: 'active-2', label: '部署机器人', status: 'active', created_at: '2026-09-02T09:23:24Z', expires_at: '2026-09-09T09:23:24Z' },
    ])
    const wrapper = mount(SystemSettingsSecurity)
    await flushPromises()

    expect(wrapper.text()).not.toContain('安全管理')
    expect(wrapper.find('.temporary-token-card .card-header').exists()).toBe(false)
    expect(wrapper.get('[data-testid="temporary-token-open-create"]').text()).toBe('生成临时秘钥')
    expect(wrapper.find('[data-testid="temporary-token-create-modal"]').exists()).toBe(false)
    expect(wrapper.find('.temporary-token-list-title').exists()).toBe(false)
    expect(wrapper.find('.temporary-token-list-count').exists()).toBe(false)
    expect(wrapper.get('.temporary-token-table-header').text()).toContain('备注')
    expect(wrapper.get('.temporary-token-table-header').text()).toContain('创建时间')
    expect(wrapper.get('.temporary-token-table-header').text()).toContain('到期时间')
    expect(wrapper.get('.temporary-token-table-header').text()).toContain('生效状态')
    expect(wrapper.get('.temporary-token-table-header').text()).toContain('操作')
    expect(wrapper.get('.temporary-token-table').find('table.data-table').exists()).toBe(true)
    expect(wrapper.get('.temporary-token-row').find('td').exists()).toBe(true)
    expect(wrapper.findAll('.temporary-token-row')).toHaveLength(2)
    const revokeButtons = wrapper.findAll('.temporary-token-action button')
    expect(revokeButtons).toHaveLength(2)
    expect(revokeButtons[0].attributes('disabled')).toBeDefined()
    expect(revokeButtons[1].attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain('供应商临时访问')
    expect(wrapper.text()).toContain('部署机器人')
    expect(wrapper.find('.temporary-token-empty').exists()).toBe(false)
    wrapper.unmount()
  })

  it('creates a temporary token in a dialog and only reveals the secret there', async () => {
    api.post.mockResolvedValueOnce({ token: 'temporary-token-once' })
    const wrapper = mount(SystemSettingsSecurity)
    await flushPromises()

    await wrapper.get('[data-testid="temporary-token-open-create"]').trigger('click')
    expect(wrapper.get('[data-testid="temporary-token-create-modal"]').text()).toContain('生成临时秘钥')
    await wrapper.get('#temporary-token-label').setValue('供应商临时访问')
    await wrapper.get('[data-testid="temporary-token-create-submit"]').trigger('click')
    await flushPromises()

    expect(api.post).toHaveBeenCalledWith('/auth/temporary-tokens', { label: '供应商临时访问', ttl_seconds: 3600 })
    expect(wrapper.get('[data-testid="temporary-token-create-modal"]').text()).toContain('仅显示一次，请立即复制')
    expect(wrapper.get('[data-testid="temporary-token-generated-secret"]').text()).toBe('temporary-token-once')
    expect(wrapper.get('.temporary-token-card').text()).not.toContain('temporary-token-once')

    await wrapper.get('[data-testid="temporary-token-create-close"]').trigger('click')
    expect(wrapper.find('[data-testid="temporary-token-create-modal"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('supports release actions on the release tab', async () => {
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await wrapper.get('[data-testid="system-settings-tab-release"]').trigger('click')
    await nextTick()

    await wrapper.get('[data-testid="platform-manual-update-open"]').trigger('click')
    await wrapper.get('[data-testid="platform-manual-image"]').setValue('registry.example.com/cylism-manager:latest')
    await wrapper.get('[data-testid="platform-manual-update"]').trigger('click')
    expect(wrapper.text()).toContain('平台更新已提交')

    await wrapper.get('[data-testid="platform-image-prefix-open"]').trigger('click')
    await wrapper.get('[data-testid="platform-image-prefix"]').setValue('registry.example.com/cylism-manager\noci-registry.example.com/cylism-manager')
    await wrapper.get('[data-testid="platform-image-prefix-save"]').trigger('click')
    expect(api.put).toHaveBeenCalledWith('/platform/image-prefix', { image_prefix: 'registry.example.com/cylism-manager\noci-registry.example.com/cylism-manager' })

    await wrapper.get('[data-testid="generate-platform-webhook-secret"]').trigger('click')
    await wrapper.get('[data-testid="platform-webhook-generate-confirm"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="platform-webhook-generated-secret"]').text()).toBe('generated-secret')
    wrapper.unmount()
  })

  it('does not show an error when a newer platform status refresh supersedes an older request', async () => {
    vi.useFakeTimers()
    route.query.tab = 'release'
    let rejectInitialRequest
    api.get
      .mockImplementationOnce((path, options) => {
        expect(path).toBe('/platform/status')
        return new Promise((resolve, reject) => {
          rejectInitialRequest = reject
          options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
        })
      })
      .mockImplementationOnce(path => {
        expect(path).toBe('/platform/status')
        return Promise.resolve({
          webhook_configured: true,
          image_prefix: 'registry.example.com/cylism-manager',
          deployment: { image: 'registry.example.com/cylism-manager:stable', ready_replicas: 1, desired_replicas: 1 },
          releases: [],
        })
      })

    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await vi.advanceTimersByTimeAsync(15000)
    await nextTick()

    expect(rejectInitialRequest).toBeTypeOf('function')
    expect(api.get).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('registry.example.com/cylism-manager:stable')
    expect(wrapper.text()).not.toContain('读取平台发布状态失败')
    wrapper.unmount()
  })

  it('shows manual platform update errors in a dialog', async () => {
    api.post.mockRejectedValueOnce(new Error('平台镜像不属于允许的仓库前缀'))
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await wrapper.get('[data-testid="system-settings-tab-release"]').trigger('click')
    await nextTick()
    await wrapper.get('[data-testid="platform-manual-update-open"]').trigger('click')
    await wrapper.get('[data-testid="platform-manual-image"]').setValue('oci-registry.crazycoding.top/cylism-manager:1.0.0')
    await wrapper.get('[data-testid="platform-manual-update"]').trigger('click')
    await nextTick()

    const notice = wrapper.find('.platform-action-notice-modal')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('平台更新失败')
    expect(notice.text()).toContain('平台镜像不属于允许的仓库前缀')
    expect(wrapper.get('[data-testid="platform-manual-update"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps release controls usable when webhook generation fails', async () => {
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await wrapper.get('[data-testid="system-settings-tab-release"]').trigger('click')
    await nextTick()
    api.post.mockRejectedValueOnce(new Error('Webhook 服务不可用'))
    await wrapper.get('[data-testid="generate-platform-webhook-secret"]').trigger('click')
    await wrapper.get('[data-testid="platform-webhook-generate-confirm"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Webhook 服务不可用')
    expect(wrapper.get('[data-testid="platform-webhook-generate-confirm"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows a local error when temporary-token creation fails', async () => {
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    api.post.mockRejectedValueOnce(new Error('临时秘钥创建失败'))
    await wrapper.get('[data-testid="temporary-token-open-create"]').trigger('click')
    await wrapper.get('[data-testid="temporary-token-create-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="temporary-token-create-modal"]').text()).toContain('临时秘钥创建失败')
    wrapper.unmount()
  })

  it('keeps unsaved endpoint fields when switching tabs and refreshes', async () => {
    vi.useFakeTimers()
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.get('[data-testid="system-settings-tab-entry"]').trigger('click')
    await wrapper.get('.domain-binding-row button').trigger('click')
    const hostname = wrapper.get('#platform-endpoint-hostname')
    await hostname.setValue('draft.example.com')

    await wrapper.get('[data-testid="system-settings-tab-security"]').trigger('click')
    await wrapper.get('[data-testid="system-settings-tab-entry"]').trigger('click')
    await vi.advanceTimersByTimeAsync(15000)
    await nextTick()

    expect(hostname.element.value).toBe('draft.example.com')
    wrapper.unmount()
  })

  it('shows a local error when the active settings read fails', async () => {
    api.get.mockImplementation(path => path === '/platform/status'
      ? Promise.reject(new Error('设置服务不可用'))
      : Promise.resolve({}))
    const wrapper = mount(SystemSettings, { global: { stubs: { Teleport: true } } })
    await nextTick()
    await wrapper.get('[data-testid="system-settings-tab-release"]').trigger('click')
    await nextTick()
    await nextTick()

    expect(wrapper.text()).toContain('设置服务不可用')
  })
})
