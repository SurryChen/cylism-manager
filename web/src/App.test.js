import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from './App.vue'
import router from './router'

const themeCss = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')

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
  document.documentElement.removeAttribute('data-palette')
})

describe('Glass UI application shell', () => {
  it('shows primary navigation in the top bar and scopes desktop secondary navigation to the active module', async () => {
    const wrapper = await mountApp()
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')
    const primaryNavigation = wrapper.get('[data-testid="primary-navigation"]')

    expect(primaryNavigation.text()).toContain('概览')
    expect(primaryNavigation.text()).toContain('基础设施')
    expect(primaryNavigation.text()).toContain('记录与系统')
    expect(navigation.text()).toContain('概览')
    expect(navigation.text()).not.toContain('服务器')
    expect(navigation.text()).not.toContain('数据管理')
  })

  it('shows infrastructure secondary navigation for infrastructure routes while preserving all mobile destinations', async () => {
    const wrapper = await mountApp('/servers')
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')
    const mobileNavigation = wrapper.get('[data-testid="mobile-navigation"]')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('基础设施')
    expect(navigation.text()).toContain('服务器')
    expect(navigation.text()).toContain('集群节点')
    expect(navigation.text()).toContain('路由')
    expect(navigation.text()).toContain('证书')
    expect(navigation.text()).toContain('工作负载')
    expect(navigation.text()).toContain('服务')
    expect(navigation.text()).toContain('配置')
    expect(navigation.text()).not.toContain('审计')
    expect(mobileNavigation.text()).toContain('概览')
    expect(mobileNavigation.text()).toContain('系统设置')
    expect(mobileNavigation.text()).not.toContain('数据管理')
  })

  it('activates only the most specific infrastructure navigation item', async () => {
    const wrapper = await mountApp('/cluster/registry-mirrors')
    const links = wrapper.get('[data-testid="desktop-navigation"]').findAll('.sidebar-link')
    const clusterNodes = links.find(link => link.text() === '集群节点')
    const registryMirrors = links.find(link => link.text() === '节点镜像源')

    expect(clusterNodes.classes()).not.toContain('is-active')
    expect(registryMirrors.classes()).toContain('is-active')
  })

  it('shows the application secondary navigation for application routes', async () => {
    const wrapper = await mountApp('/applications/1')
    const navigation = wrapper.get('[data-testid="desktop-navigation"]')

    expect(wrapper.get('[data-testid="primary-navigation"]').get('[aria-current="page"]').text()).toContain('应用')
    expect(navigation.text()).toContain('应用')
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
  })

  it('renders active navigation as floating glass controls and keeps the mobile workspace shrinkable', async () => {
    const wrapper = await mountApp('/servers')

    expect(wrapper.get('[data-testid="primary-navigation"] .is-active').classes()).toContain('topbar-nav-link')
    expect(wrapper.get('[data-testid="desktop-navigation"] .is-active').classes()).toContain('sidebar-link')
    expect(themeCss).toContain('.topbar-nav-link.is-active { align-self: center;')
    expect(themeCss).toContain('box-shadow: var(--shadow-soft), inset 0 1px 0 rgba(255,255,255,.78)')
    expect(themeCss).toContain('.sidebar-link.is-active { border-color: var(--border);')
    expect(themeCss).toContain('.topbar-nav-link { display: inline-flex; align-self: center; min-height: 34px;')
    expect(themeCss).toContain('.topbar-nav-link:hover { border-color: var(--border);')
    expect(themeCss).toContain('@media (max-width: 840px) { .app-workspace { width: 100%; min-width: 0; max-width: 100%;')
  })
})
