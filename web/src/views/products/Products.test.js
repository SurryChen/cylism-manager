import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import PersonalWorkbench from './PersonalWorkbench.vue'
import ProductCenter from './ProductCenter.vue'
import ProductDetail from './ProductDetail.vue'
import ProductEditor from './ProductEditor.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), options: vi.fn(), create: vi.fn(), update: vi.fn(), push: vi.fn() }))
vi.mock('../../api/products.js', () => ({ getProducts: mocks.list, getProduct: mocks.get, getProductResourceOptions: mocks.options, createProduct: mocks.create, updateProduct: mocks.update }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: '7' } }), useRouter: () => ({ push: mocks.push }) }))
const mountPage = component => mount(component, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true } } })
beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue([])
  mocks.options.mockResolvedValue({ projects: [], applications: [] })
  mocks.get.mockResolvedValue({ id: 7, name: '离线工具', stage: 'developing', channels: [{ id: 1, platform: 'google_play', publish_status: 'in_review', online_version: '1.0' }], projects: [{ id: 2, name: 'backend' }], applications: [] })
  mocks.create.mockResolvedValue({ id: 7 })
  mocks.update.mockResolvedValue({ id: 7 })
})

describe('personal products workflows', () => {
  it('opens a new standalone product and navigates to its detail after saving', async () => {
    const page = mountPage(ProductCenter)
    await flushPromises()
    await page.get('.product-primary').trigger('click')
    await flushPromises()
    await page.get('[aria-label="产品名称"]').setValue('离线工具')
    await page.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ name: '离线工具', stage: 'idea', project_ids: [], application_ids: [] }))
    expect(mocks.push).toHaveBeenCalledWith('/products/7')
    page.unmount()
  })

  it('keeps failed edits open and reports the server error through the shared modal', async () => {
    mocks.create.mockRejectedValue(new Error('关联资源无权访问'))
    const page = mountPage(ProductCenter)
    await flushPromises()
    await page.get('.product-primary').trigger('click')
    await flushPromises()
    await page.get('[aria-label="产品名称"]').setValue('离线工具')
    await page.get('form').trigger('submit')
    await flushPromises()
    expect(page.text()).toContain('关联资源无权访问')
    expect(page.get('[aria-label="产品名称"]').element.value).toBe('离线工具')
    expect(mocks.push).not.toHaveBeenCalled()
    page.unmount()
  })

  it('edits channels and public metadata from a reactive product without modifying the loaded data', async () => {
    const product = reactive({ id: 7, name: 'App', channels: [{ id: 1, platform: 'web', publish_status: 'published', online_version: '1.0' }] })
    const editor = mount(ProductEditor, { props: { open: true, product }, global: { stubs: { Teleport: true } } })
    await editor.findAll('.section-tab').find(button => button.text() === '发行渠道').trigger('click')
    const fields = editor.findAll('.product-form-grid input')
    await fields[2].setValue('2.0')
    await editor.findAll('.section-tab').find(button => button.text() === '公开作品').trigger('click')
    await editor.get('textarea').setValue('公开介绍')
    await editor.get('input[type="checkbox"]').setValue(true)
    await editor.get('form').trigger('submit')
    const payload = editor.emitted('save')[0][0]
    expect(payload.channels[0].online_version).toBe('2.0')
    expect(payload.public_showcase).toBe(true)
    expect(payload.public_bio).toBe('公开介绍')
    expect(product.channels[0].online_version).toBe('1.0')
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
    await page.findAll('.section-tab').find(button => button.text() === '发行渠道').trigger('click')
    await page.findAll('button').find(button => button.text() === '管理渠道').trigger('click')
    await flushPromises()
    const editor = page.getComponent(ProductEditor)
    expect(editor.get('.section-tab.is-active').text()).toBe('发行渠道')
    expect(editor.find('[aria-label="发行平台"]').exists()).toBe(true)
    expect(editor.find('[aria-label="产品名称"]').exists()).toBe(false)
    await editor.findAll('button').find(button => button.text() === '取消').trigger('click')
    await page.findAll('button').find(button => button.text() === '编辑产品').trigger('click')
    await flushPromises()
    expect(editor.get('.section-tab.is-active').text()).toBe('基本信息')
    expect(editor.get('[aria-label="产品名称"]').element.value).toBe('离线工具')
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
