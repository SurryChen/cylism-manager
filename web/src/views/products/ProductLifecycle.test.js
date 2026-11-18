import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import ProductLifecycle from './ProductLifecycle.vue'
import LifecycleEditor from './LifecycleEditor.vue'
const mocks = vi.hoisted(() => ({ page: vi.fn(), options: vi.fn(), save: vi.fn(), events: vi.fn(), import: vi.fn() }))
vi.mock('../../api/product-lifecycle.js', () => ({ getLifecyclePage: mocks.page, getLifecycleOptions: mocks.options, saveLifecycleRecord: mocks.save, getDistributionEvents: mocks.events, importProductNextAction: mocks.import }))
const product = { id: 7, name: 'App', channels: [{ id: 9, platform: 'app_store' }] }
const mountPage = kind => mount(ProductLifecycle, { props: { product, kind }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
beforeEach(() => {
 vi.clearAllMocks()
 mocks.page.mockResolvedValue({ items: [], total: 0 })
 mocks.options.mockResolvedValue({ versions: [{ id: 3, name: '1.0' }] })
 mocks.save.mockResolvedValue({ id: 10 })
 mocks.events.mockResolvedValue({ items: [{ id: 1, status: 'in_review', platform_version: '1.0', notes: 'original', created_at: '2026-11-17T00:00:00Z' }], total: 1 })
})
describe('product lifecycle workflows', () => {
 it('creates a task and preserves entered fields on a failed save', async () => {
   const page = mountPage('tasks'); await flushPromises()
   await page.findAll('button').find(button => button.text() === '新增事项').trigger('click')
   await page.get('[aria-label="事项标题"]').setValue('修复登录')
   mocks.save.mockRejectedValueOnce(new Error('记录已被更新'))
   await page.getComponent(LifecycleEditor).get('form').trigger('submit'); await flushPromises()
   expect(page.text()).toContain('记录已被更新')
   expect(page.get('[aria-label="事项标题"]').element.value).toBe('修复登录')
   expect(mocks.save).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ title: '修复登录', status: 'todo', version_id: null }))
   page.unmount()
 })
 it('uses backend pagination with default ten rows and supports changing pages', async () => {
   mocks.page.mockImplementation(async (_id, _kind, query) => ({ items: [{ id: Number(query.offset) + 1, title: 'task', status: 'todo', kind: 'action', priority: 'normal' }], total: 25 }))
   const page = mountPage('tasks'); await flushPromises()
   expect(mocks.page).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ limit: 10, offset: 0 }), expect.anything())
   await page.get('[aria-label="下一页"]').trigger('click'); await flushPromises()
   expect(mocks.page).toHaveBeenLastCalledWith(7, 'tasks', expect.objectContaining({ limit: 10, offset: 10 }), expect.anything())
   page.unmount()
 })
 it('requires versions and channels before creating a distribution', async () => {
   mocks.options.mockResolvedValue({ versions: [] })
   const page = mountPage('distributions'); await flushPromises()
   await page.findAll('button').find(button => button.text() === '新增发行记录').trigger('click')
   expect(page.text()).toContain('请先在版本页新增产品版本')
   expect(page.find('[aria-label="平台版本"]').exists()).toBe(false)
   page.unmount()
 })
 it('shows append-only distribution history', async () => {
   mocks.page.mockResolvedValue({ items: [{ id: 4, channel_id: 9, version_id: 3, platform_version: '1.0', status: 'rejected' }], total: 1 })
   const page = mountPage('distributions'); await flushPromises()
   await page.get('[aria-label="发行历史"]').trigger('click'); await flushPromises()
   expect(mocks.events).toHaveBeenCalledWith(7, 4, expect.objectContaining({ limit: 10 }), expect.anything())
   expect(page.text()).toContain('original')
   expect(page.text()).toContain('审核中')
   page.unmount()
 })
 it('edits a version without mutating reactive source and keeps hidden-page deployment selections', async () => {
   const record = reactive({ id: 1, name: '1.0', status: 'testing', deployment_ids: [99] })
   const page = mount(LifecycleEditor, { props: { open: true, kind: 'versions', productId: 7, record }, global: { stubs: { Teleport: true } } }); await flushPromises()
   await page.get('[aria-label="版本名称"]').setValue('1.1')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[0][0]).toMatchObject({ name: '1.1', deployment_ids: [99] })
   expect(record.name).toBe('1.0')
   page.unmount()
 })
})
