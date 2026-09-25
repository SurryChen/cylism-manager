import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from './App.vue'
import router from './router'

const themeCss = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')
const componentsCss = readFileSync(resolve(process.cwd(), 'src/styles/components.css'), 'utf8')
const legacyCardConsumers = [
  'views/Dashboard.vue',
  'views/DBAdmin.vue',
  'views/Monitoring.vue',
  'views/RuntimeManagement.vue',
  'views/applications/ApplicationDetails.vue',
  'views/applications/Applications.vue',
  'views/applications/Domains.vue',
  'views/applications/ImageRegistries.vue',
  'views/applications/ProjectEnvironments.vue',
  'views/applications/ReleaseDetails.vue',
  'views/cluster/ChartRepositories.vue',
  'views/cluster/Cluster.vue',
  'views/cluster/ClusterDNS.vue',
  'views/cluster/PersistentVolumes.vue',
  'views/cluster/Servers.vue',
  'views/cluster/SystemComponents.vue',
  'views/monitoring/AlertingWorkspace.vue',
  'views/monitoring/DiskGrowthWorkspace.vue',
  'views/monitoring/LoggingWorkspace.vue',
  'views/monitoring/MetricTrendChart.vue',
  'views/network/CertificateOperations.vue',
  'views/network/Certificates.vue',
  'views/network/Sites.vue',
  'views/resources/Configs.vue',
  'views/resources/Resources.vue',
  'views/resources/Services.vue',
  'views/resources/Workloads.vue',
].map(path => readFileSync(resolve(process.cwd(), 'src', path), 'utf8'))

async function mountApp(path = '/') {
  localStorage.setItem('access_token', 'test-token')
  await router.push(path)
  await router.isReady()
  return mount(App, {
    global: {
      plugins: [router],
      stubs: { RouterView: true },
    },
  })
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.dataset.palette = 'mint'
  document.body.innerHTML = ''
})

afterEach(() => {
  vi.useRealTimers()
  document.documentElement.removeAttribute('data-palette')
})

describe('Glass UI application shell', () => {
  it('does not render static environment or agent connection status', async () => {
    const wrapper = await mountApp('/servers')

    expect(wrapper.find('.environment-status').exists()).toBe(false)
    expect(wrapper.find('.sidebar-status').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Production')
    expect(wrapper.text()).not.toContain('平台在线')
    expect(wrapper.text()).not.toContain('agents connected')
  })

  it('uses a full-width workspace for the overview while keeping primary navigation in the top bar', async () => {
    const wrapper = await mountApp()
    const primaryNavigation = wrapper.get('[data-testid="primary-navigation"]')

    expect(primaryNavigation.text()).toContain('概览')
    expect(primaryNavigation.text()).toContain('基础设施')
    expect(primaryNavigation.text()).toContain('记录与系统')
    expect(wrapper.find('[data-testid="desktop-navigation"]').exists()).toBe(false)
    expect(wrapper.get('.app-workspace').classes()).toContain('app-workspace--wide')
  })

  it('places the Registry workspace in the cloud services sidebar and removes delivery from top navigation', async () => {
    const wrapper = await mountApp('/cloud-services/registry')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('云服务')
    expect(wrapper.get('[data-testid="primary-navigation"]').text()).not.toContain('交付中心')
    expect(wrapper.get('[data-testid="desktop-navigation"]').get('.sidebar-link.is-active').text()).toContain('制品库')
    expect(wrapper.get('.app-workspace').classes()).not.toContain('app-workspace--wide')
  })

  it('shows infrastructure secondary navigation for infrastructure routes while preserving all mobile destinations', async () => {
    const wrapper = await mountApp('/servers')
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')
    const mobileNavigation = wrapper.get('[data-testid="mobile-navigation"]')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('基础设施')
    expect(navigation.find('.sidebar-context').exists()).toBe(false)
    expect(navigation.text()).toContain('服务器')
    expect(navigation.text()).toContain('集群')
    expect(navigation.text()).toContain('Kubernetes 资源')
    expect(navigation.text()).toContain('网络访问')
    expect(navigation.text()).toContain('存储')
    expect(navigation.text()).toContain('监控')
    expect(navigation.text()).toContain('Agent 助手')
    expect(navigation.text()).not.toContain('节点镜像源')
    expect(navigation.text()).not.toContain('Chart 仓库')
    expect(navigation.text()).not.toContain('工作负载')
    expect(navigation.text()).not.toContain('配置')
    expect(navigation.text()).not.toContain('审计')
    expect(mobileNavigation.text()).toContain('概览')
    expect(mobileNavigation.text()).toContain('系统设置')
    expect(mobileNavigation.text()).not.toContain('数据管理')
  })

  it.each([
    ['/audit', '审计日志'],
    ['/operations', '操作历史'],
    ['/settings/system', '系统设置'],
  ])('keeps %s as a direct records-and-system sidebar route', async (path, label) => {
    const wrapper = await mountApp(path)
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')

    expect(router.currentRoute.value.path).toBe(path)
    expect(navigation.get('.sidebar-link.is-active').text()).toContain(label)
    expect(navigation.findAll('.sidebar-link.is-active')).toHaveLength(1)
  })

  it('activates the cluster aggregation entry for a cluster configuration route', async () => {
    const wrapper = await mountApp('/cluster/registry-mirrors')
    const links = wrapper.get('[data-testid="desktop-navigation"]').findAll('.sidebar-link')
    const cluster = links.find(link => link.text() === '集群')

    expect(cluster.classes()).toContain('is-active')
  })

  it('shows the application secondary navigation for application routes', async () => {
    const wrapper = await mountApp('/applications/1')
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('应用')
    expect(navigation.text()).toContain('项目与环境')
    expect(navigation.text()).toContain('镜像仓库')
    expect(navigation.text()).toContain('工作台')
    expect(navigation.text()).toContain('全局概览')
    expect(navigation.text()).not.toContain('应用栈')
    expect(navigation.get('.sidebar-link.is-active').text()).toContain('工作台')
  })

  it('keeps domain drill-down routes in the application navigation group', async () => {
    const wrapper = await mountApp('/applications/domains?project_id=1&environment_id=1')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('应用')
    expect(wrapper.get('[data-testid="desktop-navigation"]').get('.sidebar-link.is-active').text()).toContain('工作台')
  })

  it('renders the palette menu at the document root and applies a selected swatch', async () => {
    const wrapper = await mountApp()

    await wrapper.get('[aria-label="选择配色"]') .trigger('click')
    await flushPromises()

    const menu = document.body.querySelector('[data-testid="palette-menu"]')
    expect(menu).not.toBeNull()
    expect(menu.parentElement).toBe(document.body)

    await menu.querySelector('[data-palette-option="sky"]').click()
    await flushPromises()

    expect(document.documentElement.dataset.palette).toBe('sky')
    expect(localStorage.getItem('cylism-palette')).toBe('sky')
  })

  it('opens and closes the mobile navigation drawer', async () => {
    const wrapper = await mountApp()

    await wrapper.get('[aria-label="打开导航菜单"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="mobile-navigation"]').classes()).toContain('is-open')

    await wrapper.get('[aria-label="关闭导航菜单"]').trigger('click')
    expect(wrapper.get('[data-testid="mobile-navigation"]').classes()).not.toContain('is-open')
  })

  it('keeps long mobile navigation lists scrollable within the drawer', () => {
    expect(themeCss).toMatch(/\.mobile-drawer \.navigation-groups\s*\{[^}]*flex:\s*1[^}]*min-height:\s*0[^}]*overflow-y:\s*auto/)
  })

  it('shows the mobile drawer scrollbar only while its navigation is scrolling', async () => {
    vi.useFakeTimers()
    const wrapper = await mountApp()
    document.body.append(wrapper.element)
    const navigation = wrapper.get('[data-testid="mobile-navigation"] .navigation-groups')

    expect(navigation.classes()).not.toContain('is-scrolling')

    await navigation.trigger('scroll')
    expect(navigation.classes()).toContain('is-scrolling')

    await vi.advanceTimersByTimeAsync(700)
    expect(navigation.classes()).not.toContain('is-scrolling')
    wrapper.unmount()
  })

  it('installs transient scrollbar handling for document and nested scroll targets', async () => {
    vi.useFakeTimers()
    const wrapper = await mountApp()
    const scrollRegion = document.createElement('div')
    document.body.append(scrollRegion)

    document.dispatchEvent(new Event('scroll'))
    scrollRegion.dispatchEvent(new Event('scroll'))

    expect(document.documentElement.classList).toContain('is-scrolling')
    expect(scrollRegion.classList).toContain('is-scrolling')

    wrapper.unmount()
    expect(document.documentElement.classList).not.toContain('is-scrolling')
    expect(scrollRegion.classList).not.toContain('is-scrolling')
  })

  it('keeps scrollbars transparent until a scroll target is active', () => {
    expect(themeCss).toMatch(/\*\s*\{[^}]*scrollbar-color:\s*transparent transparent/)
    expect(themeCss).toMatch(/\.is-scrolling\s*\{[^}]*scrollbar-color:\s*var\(--text-muted\) transparent/)
    expect(themeCss).toMatch(/::-webkit-scrollbar-thumb\s*\{[^}]*background:\s*transparent/)
    expect(themeCss).toMatch(/\.is-scrolling::-webkit-scrollbar-thumb\s*\{[^}]*background:\s*var\(--text-muted\)/)
  })

  it('uses the Direction 05 薄荷玻璃 palette and an unframed desktop top bar', async () => {
    const wrapper = await mountApp()

    await wrapper.get('[aria-label="选择配色"]').trigger('click')
    await flushPromises()

    expect(document.body.querySelector('[data-testid="palette-menu"]').textContent).toContain('薄荷玻璃')
    expect(themeCss).toContain('--canvas: #eaf1f0')
    expect(themeCss).toContain('--band-a: #c2e4db')
    expect(themeCss).toContain('--band-b: #d8e4f7')
    expect(themeCss).toContain('--action-primary: #22736b')
    expect(themeCss).toMatch(/\.app-topbar\s*\{[^}]*border:\s*0[^}]*border-bottom:\s*1px solid var\(--border-muted\)[^}]*border-radius:\s*0/)
    expect(themeCss).toContain('width: 85vw')
    expect(themeCss).toContain('height: 58vh')
    expect(componentsCss).not.toContain('当前工作区')
    expect(themeCss).toContain('--page-header-height: 64px')
    expect(themeCss).toContain('--tabbed-page-header-height: var(--page-header-height)')
    expect(componentsCss).toContain('height: var(--tabbed-page-header-height)')
  })

  it('renders active navigation as floating glass controls and keeps the mobile workspace shrinkable', async () => {
    const wrapper = await mountApp('/servers')

    expect(wrapper.get('[data-testid="primary-navigation"] .is-active').classes()).toContain('topbar-nav-link')
    expect(wrapper.get('[data-testid="desktop-navigation"] .is-active').classes()).toContain('sidebar-link')
    expect(themeCss).toContain('.topbar-nav-link.is-active { align-self: center;')
    expect(themeCss).toContain('box-shadow: var(--shadow-soft), inset 0 1px 0 rgba(255,255,255,.78)')
    expect(themeCss).toContain('.sidebar-link.is-active { border-color: var(--border);')
    expect(themeCss).toContain('transition: background .18s ease, color .18s ease, border-color .18s ease, transform .18s ease, box-shadow .18s ease')
    expect(themeCss).toContain('.topbar-nav-link { display: inline-flex; align-self: center; min-height: 34px;')
    expect(themeCss).toContain('.topbar-nav-link:hover { border-color: var(--border);')
    expect(themeCss).toContain('@media (max-width: 840px) { .app-workspace { width: 100%; min-width: 0; max-width: 100%;')
  })

  it('keeps text buttons on one line when an operational table is constrained', () => {
    expect(componentsCss).toMatch(/\.btn\s*\{[^}]*white-space:\s*nowrap/)
  })

  it('keeps compact status badges on one line when an operational table is constrained', () => {
    expect(componentsCss).toMatch(/\.badge\s*\{[^}]*white-space:\s*nowrap/)
  })

  it('does not retain legacy card outer containers in operational views', () => {
    expect(legacyCardConsumers.join('\n')).not.toMatch(/class=["'](?:[^"']*\s)?card(?:\s|["'])/)
  })

  it('retires the legacy card surface while retaining metric, modal, and banner styling', () => {
    expect(componentsCss).not.toMatch(/\.card(?:[\s,{:]|$)/)
    expect(componentsCss).toMatch(/\.metric(?:[\s,{:]|$)/)
    expect(componentsCss).toMatch(/\.modal(?:[\s,{:]|$)/)
    expect(componentsCss).toMatch(/\.k8s-banner(?:[\s,{:]|$)/)
  })
})
