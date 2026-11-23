import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import PersonalWorkbench from './PersonalWorkbench.vue'
import ProductCenter from './ProductCenter.vue'
import ProductDetail from './ProductDetail.vue'
import ProductEditor from './ProductEditor.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), page: vi.fn(), get: vi.fn(), options: vi.fn(), create: vi.fn(), update: vi.fn(), push: vi.fn(), replace: vi.fn(), workbench: vi.fn() }))
vi.mock('../../api/products.js', () => ({ getProducts: mocks.list, getProductPage: mocks.page, getProduct: mocks.get, getProductResourceOptions: mocks.options, createProduct: mocks.create, updateProduct: mocks.update }))
vi.mock('../../api/product-lifecycle.js', () => ({ getProductWorkbench: mocks.workbench }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ push: mocks.push, replace: mocks.replace }) }))
const route = reactive({ params: { id: '7' }, query: {} })
const mountPage = component => mount(component, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, ProductLifecycle: { props: ['kind'], template: '<div class="lifecycle-stub" :data-kind="kind" />' }, Teleport: true } } })
beforeEach(() => {
  vi.clearAllMocks()
  route.params.id = '7'
  route.query = {}
  mocks.list.mockResolvedValue([])
  mocks.workbench.mockImplementation(async () => ({ products: await mocks.list(), tasks: [], versions: [], distributions: [] }))
  mocks.page.mockResolvedValue({ items: [], total: 0 })
  mocks.options.mockResolvedValue({ projects: [], applications: [] })
  mocks.get.mockResolvedValue({ id: 7, name: '离线工具', stage: 'developing', channels: [{ id: 1, platform: 'google_play', publish_status: 'in_review', online_version: '1.0' }], projects: [{ id: 2, name: 'backend' }], applications: [] })
  mocks.create.mockResolvedValue({ id: 7 })
  mocks.update.mockResolvedValue({ id: 7 })
})

describe('personal products workflows', () => {
  it('associates the current management platform without a project or application', async () => {
    const editor = mount(ProductEditor, { props: { open: true, product: { id: 7, name: 'Cylism Manager' }, initialTab: 'resources', options: { projects: [], applications: [], platform: { name: 'Cylism Manager（当前管理平台）' } } }, global: { stubs: { Teleport: true } } })
    await editor.get('[aria-label="关联当前管理平台"]').setValue(true)
    await editor.get('form').trigger('submit')
    expect(editor.emitted('save')[0][0]).toMatchObject({ platform_linked: true, project_ids: [], application_ids: [] })
    await editor.get('[aria-label="关联当前管理平台"]').setValue(false)
    await editor.get('form').trigger('submit')
    expect(editor.emitted('save')[1][0].platform_linked).toBe(false)
    editor.unmount()
  })

  it('shows platform navigation and preserves self association in basic edits', async () => {
    mocks.get.mockResolvedValue({ id: 7, name: 'Cylism Manager', stage: 'operating', platform_linked: true })
    const page = mountPage(ProductDetail)
    await flushPromises()
    expect(page.find('.product-disclosure summary').text()).toContain('当前管理平台')
    expect(page.text()).toContain('发布与更新')
    await page.findAll('button').find(button => button.text() === '编辑基本信息').trigger('click')
    await page.get('[aria-label="产品名称"]').setValue('Manager')
    await page.getComponent(ProductEditor).get('form').trigger('submit')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(7, expect.objectContaining({ name: 'Manager', platform_linked: true }))
    page.unmount()
  })

  it('opens a new standalone product and navigates to its detail after saving', async () => {
    const page = mountPage(ProductCenter)
    await flushPromises()
    await page.get('.product-primary').trigger('click')
    await flushPromises()
    await page.get('[aria-label="产品名称"]').setValue('离线工具')
    await page.getComponent(ProductEditor).get('form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ name: '离线工具', stage: 'idea', project_ids: [], application_ids: [] }))
    expect(mocks.push).toHaveBeenCalledWith('/products/7')
    expect(mocks.options).not.toHaveBeenCalled()
    page.unmount()
  })

  it('keeps failed edits open and reports the server error through the shared modal', async () => {
    mocks.create.mockRejectedValue(new Error('关联资源无权访问'))
    const page = mountPage(ProductCenter)
    await flushPromises()
    await page.get('.product-primary').trigger('click')
    await flushPromises()
    await page.get('[aria-label="产品名称"]').setValue('离线工具')
    await page.getComponent(ProductEditor).get('form').trigger('submit')
    await flushPromises()
    expect(page.text()).toContain('关联资源无权访问')
    expect(page.get('[aria-label="产品名称"]').element.value).toBe('离线工具')
    expect(mocks.push).not.toHaveBeenCalled()
    page.unmount()
  })

  it('removes product-level action inputs without clearing legacy data when saving other fields', async () => {
    const product = reactive({ id: 7, name: 'App', next_action: '跟进商店审核', next_due_date: '2026-12-01' })
    const editor = mount(ProductEditor, { props: { open: true, product }, global: { stubs: { Teleport: true } } })
    expect(editor.text()).not.toContain('下一步行动')
    expect(editor.text()).not.toContain('截止日期')
    expect(editor.find('input[type="date"]').exists()).toBe(false)
    await editor.get('[aria-label="产品名称"]').setValue('Updated App')
    await editor.get('form').trigger('submit')
    expect(editor.emitted('save')[0][0]).toEqual(expect.objectContaining({
      name: 'Updated App', next_action: '跟进商店审核', next_due_date: '2026-12-01',
    }))
    expect(product.name).toBe('App')
    expect(product.next_action).toBe('跟进商店审核')
    expect(product.next_due_date).toBe('2026-12-01')
    editor.unmount()
  })

  it('edits only the requested section and preserves all other product data', async () => {
    const product = reactive({ id: 7, name: 'App', channels: [{ id: 1, platform: 'web', publish_status: 'published', online_version: '1.0' }], public_bio: 'Existing bio', projects: [{ id: 2 }], applications: [{ id: 4 }] })
    const editor = mount(ProductEditor, { props: { open: true, product, initialTab: 'showcase' }, global: { stubs: { Teleport: true } } })
    expect(editor.findAll('.section-tab').map(button => button.text())).toEqual(['公开资料'])
    expect(editor.find('[aria-label="产品名称"]').exists()).toBe(false)
    expect(editor.find('[aria-label="发行平台"]').exists()).toBe(false)
    await editor.get('textarea').setValue('公开介绍')
    await editor.get('input[type="checkbox"]').setValue(true)
    await editor.get('form').trigger('submit')
    const payload = editor.emitted('save')[0][0]
    expect(payload).toMatchObject({ name: 'App', public_showcase: true, public_bio: '公开介绍', project_ids: [2], application_ids: [4], channels: [{ id: 1, online_version: '1.0', publish_status: 'published' }] })
    expect(product.public_bio).toBe('Existing bio')
    expect(product.channels[0].online_version).toBe('1.0')
    editor.unmount()
  })

  it('keeps channel progress read-only while allowing pause and resume without losing metadata', async () => {
    const product = reactive({ id: 7, name: 'App', channels: [{ id: 1, platform: 'web', name: 'Website', publish_status: 'in_review', online_version: '1.0', pending_version: '1.1' }] })
    const editor = mount(ProductEditor, { props: { open: true, product, initialTab: 'channels' }, global: { stubs: { Teleport: true } } })
    expect(editor.findAll('.section-tab').map(button => button.text())).toEqual(['发行渠道'])
    expect(editor.find('[aria-label="发行状态"]').exists()).toBe(false)
    await editor.get('[aria-label="暂停发行"]').setValue(true)
    await editor.get('form').trigger('submit')
    expect(editor.emitted('save')[0][0].channels[0]).toMatchObject({ publish_status: 'paused', online_version: '1.0', pending_version: '1.1' })
    await editor.get('[aria-label="暂停发行"]').setValue(false)
    await editor.get('form').trigger('submit')
    expect(editor.emitted('save')[1][0].channels[0].publish_status).toBe('in_review')
    expect(product.channels[0].publish_status).toBe('in_review')
    editor.unmount()
  })

  it('archives after confirmation and preserves channels and resource links', async () => {
    const page = mountPage(ProductDetail)
    await flushPromises()
    await page.findAll('button').find(button => button.text() === '归档').trigger('click')
    expect(mocks.update).not.toHaveBeenCalled()
    await page.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(7, expect.objectContaining({ stage: 'archived', project_ids: [2], channels: [expect.objectContaining({ id: 1, online_version: '1.0' })] }))
    page.unmount()
  })

  it('opens channel management directly in the channel editor', async () => {
    const page = mountPage(ProductDetail)
    await flushPromises()
    await page.findAll('.section-tab').find(button => button.text() === '发行').trigger('click')
    await page.findAll('.section-tab').find(button => button.text() === '发行渠道').trigger('click')
    expect(mocks.replace).toHaveBeenCalledWith({ query: { tab: 'channels' } })
    await page.findAll('button').find(button => button.text() === '管理渠道').trigger('click')
    await flushPromises()
    const editor = page.getComponent(ProductEditor)
    expect(editor.get('.section-tab.is-active').text()).toBe('发行渠道')
    expect(editor.find('[aria-label="发行平台"]').exists()).toBe(true)
    expect(editor.find('[aria-label="产品名称"]').exists()).toBe(false)
    await editor.findAll('button').find(button => button.text() === '取消').trigger('click')
    await page.findAll('button').find(button => button.text() === '编辑基本信息').trigger('click')
    await flushPromises()
    expect(editor.findAll('.section-tab').map(button => button.text())).toEqual(['基本信息'])
    expect(editor.get('[aria-label="产品名称"]').element.value).toBe('离线工具')
    expect(mocks.options).not.toHaveBeenCalled()
    page.unmount()
  })

  it('shows four primary tabs and opens links and optional settings in their own editors', async () => {
    const page = mountPage(ProductDetail)
    await flushPromises()
    expect(page.findAll('.section-tab').map(button => button.text())).toEqual(['总览', '事项', '版本', '发行'])
    expect(page.text()).not.toContain('研发与交付')
    expect(page.findAll('.product-disclosure').every(section => !section.element.open)).toBe(true)
    for (const [buttonLabel, section] of [['管理链接', '资料链接'], ['编辑公开资料', '公开资料'], ['管理关联', '资源关联']]) {
      await page.findAll('button').find(button => button.text() === buttonLabel).trigger('click')
      await flushPromises()
      const editor = page.getComponent(ProductEditor)
      expect(editor.findAll('.section-tab').map(button => button.text())).toEqual([section])
      expect(editor.find('[aria-label="产品名称"]').exists()).toBe(false)
      await editor.findAll('button').find(button => button.text() === '取消').trigger('click')
    }
    expect(mocks.options).toHaveBeenCalledTimes(1)
    page.unmount()
  })

  it.each([['channels', 'distributions'], ['resources', 'overview'], ['showcase', 'overview']])('preserves existing %s deep links', async (legacyTab, primaryTab) => {
    route.query = { tab: legacyTab }
    const page = mountPage(ProductDetail)
    await flushPromises()
    expect(page.find('.section-tabs-header .section-tab.is-active').text()).toBe(primaryTab === 'overview' ? '总览' : '发行')
    if (legacyTab === 'channels') expect(page.findAll('.section-tab.is-active').map(button => button.text())).toEqual(['发行', '发行渠道'])
    else expect(page.findAll('.product-disclosure')[legacyTab === 'resources' ? 0 : 1].element.open).toBe(true)
    page.unmount()
  })

  it('summarizes owned products and exposes actionable channel details without fake operational data', async () => {
    mocks.list.mockResolvedValue([
      { id: 7, name: '活跃产品', stage: 'developing', next_action: '修复审核问题', next_due_date: '2000-01-01', channels: [{ id: 1, platform: 'app_store', publish_status: 'rejected' }] },
      { id: 8, name: '已归档作品', stage: 'archived', next_action: '隐藏行动' },
    ])
    const page = mountPage(PersonalWorkbench)
    await flushPromises()
    expect(page.text()).toContain('修复审核问题')
    expect(page.text()).toContain('已逾期')
    expect(page.text()).toContain('审核拒绝')
    expect(page.text()).not.toContain('隐藏行动')
    expect(page.text()).not.toContain('运行正常')
    page.unmount()
  })

  it('shows failures separately from an empty successful workbench', async () => {
    mocks.list.mockRejectedValue(new Error('网络不可用'))
    const page = mountPage(PersonalWorkbench)
    await flushPromises()
    expect(page.text()).toContain('网络不可用')
    expect(page.text()).not.toContain('还没有活跃产品')
    expect(page.find('.product-stat-grid').exists()).toBe(false)
    page.unmount()
  })
})
