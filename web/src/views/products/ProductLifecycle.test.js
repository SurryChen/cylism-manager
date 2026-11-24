import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import ProductLifecycle from './ProductLifecycle.vue'
import LifecycleEditor from './LifecycleEditor.vue'
import { deploymentCount, deploymentPath } from './lifecycle-utils.js'
const mocks = vi.hoisted(() => ({ page: vi.fn(), options: vi.fn(), save: vi.fn(), remove: vi.fn(), events: vi.fn(), import: vi.fn() }))
vi.mock('../../api/product-lifecycle.js', () => ({ getLifecyclePage: mocks.page, getLifecycleOptions: mocks.options, saveLifecycleRecord: mocks.save, deleteProductTask: mocks.remove, getDistributionEvents: mocks.events, importProductNextAction: mocks.import }))
const product = { id: 7, name: 'App', channels: [{ id: 9, platform: 'app_store' }] }
const mountPage = kind => mount(ProductLifecycle, { props: { product, kind }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
beforeEach(() => {
 vi.clearAllMocks()
 mocks.page.mockResolvedValue({ items: [], total: 0 })
 mocks.options.mockResolvedValue({ versions: [{ id: 3, name: '1.0' }] })
 mocks.save.mockResolvedValue({ id: 10 })
 mocks.remove.mockResolvedValue(null)
 mocks.events.mockResolvedValue({ items: [{ id: 1, status: 'in_review', platform_version: '1.0', notes: 'original', created_at: '2026-11-17T00:00:00Z' }], total: 1 })
})
describe('product lifecycle workflows', () => {
 it('opens channel details from the release-record toolbar without changing the list', async () => {
   mocks.page.mockResolvedValue({ items: [{ id: 4, version_id: 3, channel_id: 9, platform_version: '1.0', status: 'published' }], total: 1 })
   const page = mountPage('distributions'); await flushPromises()
   expect(page.get('.data-table').findAll('th').map(cell => cell.text())).toEqual(['产品版本', '发行渠道 / 分类', '状态', '更新时间 / 备注', '操作'])
   expect(page.get('.data-table tbody td').text()).toBe('1.0')
   await page.findAll('button').find(button => button.text() === '发行渠道').trigger('click')
   expect(page.emitted('show-channels')).toHaveLength(1)
   expect(page.get('.data-table').exists()).toBe(true)
   page.unmount()
 })
 it('keeps version rows concise and preserves historical fields when editing', async () => {
   const historicalFields = { goal: '简化产品版本列表', acceptance: '部署结果已验证', target_date: '2026-12-01', repository_url: 'https://example.com/repository', commit_sha: '123456789abcdef123456789abcdef123456789ab', build_url: 'https://example.com/build/8', artifact_url: 'https://example.com/artifact/8' }
   const record = { id: 8, name: 'dev-123456789abc', status: 'completed', notes: '根据已有平台更新补录；原部署时间和镜像信息保留。', platform_release_ids: [4], ...historicalFields }
   mocks.page.mockImplementation(async (_id, kind) => kind === 'versions'
     ? { items: [record], total: 1 }
     : { items: [], total: 0 })
   const page = mountPage('versions'); await flushPromises()
   const table = page.get('.data-table')
   expect(table.findAll('th').map(cell => cell.text())).toEqual(['版本', '说明', '发版日期', '关联部署', '操作'])
   const cells = table.findAll('tbody td')
   expect(cells[0].text()).toBe(record.name)
   expect(cells[1].get('.overflow-tooltip-trigger--multiline').text()).toBe(record.notes)
   for (const value of [record.acceptance, record.goal, record.repository_url, record.commit_sha, '未设目标日期', '未填写验收项']) expect(table.text()).not.toContain(value)
   await page.get('[aria-label="查看版本详情"]').trigger('click')
   const detail = page.get('.product-detail-list')
   for (const value of [record.name, record.acceptance, record.notes, record.commit_sha]) expect(page.text()).toContain(value)
   for (const url of [record.repository_url, record.build_url, record.artifact_url]) expect(detail.findAll('a').map(link => link.attributes('href'))).toContain(url)
   expect(detail.text()).toContain(record.target_date)
   expect(mocks.save).not.toHaveBeenCalled()
   await page.setProps({ product: { ...product, id: 9 } }); await flushPromises()
   expect(page.find('.product-detail-list').exists()).toBe(false)
   await page.get('[aria-label="编辑版本"]').trigger('click'); await flushPromises()
   const editor = page.getComponent(LifecycleEditor)
   expect(editor.get('input[required]').element.value).toBe(record.name)
   expect(editor.get('[aria-label="版本说明"]').element.value).toBe(record.notes)
   expect(editor.findAll('label.form-group').map(field => field.get('.form-label').text())).toEqual(['版本号 *', '正式发版日期（历史记录可留空）', '版本说明（可选）'])
   await editor.get('[aria-label="版本说明"]').setValue('更新版本说明')
   await editor.get('form').trigger('submit'); await flushPromises()
   expect(mocks.save).toHaveBeenCalledWith(9, 'versions', expect.objectContaining({ ...historicalFields, notes: '更新版本说明', platform_release_ids: [4] }))
   page.unmount()
 })
 it('omits empty version fields and retains planned dates and task navigation', async () => {
   mocks.page.mockResolvedValue({ items: [{ id: 8, name: '1.2', status: 'planning', target_date: '2026-12-01' }], total: 1 })
   const page = mountPage('versions'); await flushPromises()
   expect(page.get('.data-table').text()).not.toContain('2026-12-01')
   expect(page.find('.lifecycle-version-preview').exists()).toBe(false)
   expect(page.findAll('button').some(button => button.text() === '0 条记录')).toBe(false)
   await page.get('[aria-label="查看版本事项"]').trigger('click')
   expect(page.emitted('select-version')[0]).toEqual([8])
   await page.get('[aria-label="查看版本详情"]').trigger('click')
   expect(page.get('.product-detail-list').text()).toContain('2026-12-01')
   expect(page.get('.product-detail-list').findAll('dt').map(item => item.text())).toEqual(['历史状态', '目标日期'])
   page.unmount()
 })
 it('keeps application and platform selections separate even when their IDs match', async () => {
   mocks.page.mockImplementation(async (_id, kind) => kind === 'deployments'
     ? { items: [{ id: 1, source: 'application', application_name: 'API', version: '1.0' }, { id: 1, source: 'platform', application_name: 'Cylism Manager', version: 'manager:1.1' }], total: 2 }
     : { items: [], total: 0 })
   const page = mount(LifecycleEditor, { props: { open: true, kind: 'versions', productId: 7 }, global: { stubs: { Teleport: true } } }); await flushPromises()
   expect(mocks.page).not.toHaveBeenCalledWith(7, 'deployments', expect.anything(), expect.anything())
   const toggle = page.get('button[aria-controls][aria-expanded="false"]')
   expect(toggle.text()).toBe('关联部署记录')
   await toggle.trigger('click'); await flushPromises()
   const checkboxes = page.findAll('input[type="checkbox"]')
   await checkboxes[0].setValue(true)
   await checkboxes[1].setValue(true)
   expect(page.text()).toContain('已选择 2 条')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[0][0]).toMatchObject({ deployment_ids: [1], platform_release_ids: [1] })
   await checkboxes[0].setValue(false)
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[1][0]).toMatchObject({ deployment_ids: [], platform_release_ids: [1] })
   await page.findAll('button').find(button => button.text() === '清空选择').trigger('click')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[2][0]).toMatchObject({ deployment_ids: [], platform_release_ids: [] })
   expect(deploymentCount({ platform_release_ids: [1] })).toBe(1)
   expect(deploymentPath({ source: 'platform', id: 1 })).toBe('/settings/system?tab=release')
   expect(deploymentPath({ source: 'application', id: 1, application_id: 2 })).toBe('/applications/2/releases/1')
   page.unmount()
 })
 it('opens platform-only version history and links to platform settings', async () => {
   mocks.page.mockImplementation(async (_id, kind) => kind === 'versions' ? { items: [{ id: 8, name: '1.1', status: 'testing', platform_release_ids: [1] }], total: 1 } : { items: [{ id: 1, source: 'platform', application_name: 'Cylism Manager', version: 'manager:1.1' }], total: 1 })
   const page = mountPage('versions'); await flushPromises()
   const button = page.findAll('button').find(button => button.text() === '1 条记录')
   expect(button.attributes('disabled')).toBeUndefined()
   await button.trigger('click'); await flushPromises()
   expect(mocks.page).toHaveBeenCalledWith(7, 'deployments', expect.objectContaining({ version_id: 8 }), expect.anything())
   expect(page.text()).toContain('平台发布与更新')
   await page.findAll('button').find(button => button.text() === '编辑关联').trigger('click'); await flushPromises()
   expect(page.get('[aria-label="搜索部署记录"]').exists()).toBe(true)
   page.unmount()
 })
 it('opens the deployment picker directly from an unlinked version', async () => {
   const version = { id: 8, name: '1.1', goal: '历史目标', status: 'completed' }
   mocks.page.mockImplementation(async (_id, kind) => {
     if (kind === 'versions') return { items: [version], total: 1 }
     if (kind === 'deployments') return { items: [{ id: 6, application_name: 'API', environment_kind: 'production' }], total: 1 }
     return { items: [], total: 0 }
   })
   const page = mountPage('versions'); await flushPromises()
   const table = page.get('.data-table')
   expect(table.get('.lifecycle-version-preview').text()).toBe(version.goal)
   await table.findAll('button').find(button => button.text() === '关联部署').trigger('click'); await flushPromises()
   const editor = page.getComponent(LifecycleEditor)
   expect(editor.get('[aria-label="搜索部署记录"]').exists()).toBe(true)
   await editor.get('input[type="checkbox"]').setValue(true)
   await editor.get('form').trigger('submit'); await flushPromises()
   expect(mocks.save).toHaveBeenCalledWith(7, 'versions', expect.objectContaining({ id: 8, deployment_ids: [6] }))
   page.unmount()
 })
 it('creates a task and preserves entered fields on a failed save', async () => {
   const page = mountPage('tasks'); await flushPromises()
   await page.findAll('button').find(button => button.text() === '新增事项').trigger('click')
   expect(page.find('select[aria-label="关联产品版本"]').exists()).toBe(false)
   await page.get('[aria-label="事项标题"]').setValue('修复登录')
   mocks.save.mockRejectedValueOnce(new Error('记录已被更新'))
   await page.getComponent(LifecycleEditor).get('form').trigger('submit'); await flushPromises()
   expect(page.text()).toContain('记录已被更新')
   expect(page.get('[aria-label="事项标题"]').element.value).toBe('修复登录')
   expect(mocks.save).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ title: '修复登录', status: 'todo', version_id: null }))
   page.unmount()
 })
 it('shows task details and allows linking multiple releases when editing', async () => {
   const task = { id: 8, title: '部署验证', details: '验证环境配置\n检查发布结果', kind: 'action', priority: 'normal', status: 'todo', version_ids: [3] }
   mocks.page.mockResolvedValue({ items: [task], total: 1 })
   mocks.options.mockResolvedValue({ versions: [{ id: 3, name: '1.0' }, { id: 5, name: '2.0' }] })
   const page = mount(ProductLifecycle, { props: { product, kind: 'tasks' }, attachTo: document.body })
   await flushPromises()
   const body = new DOMWrapper(document.body)
   try {
     expect(page.get('.overflow-tooltip-trigger').attributes('aria-label')).toBe(task.details)
     await page.get('[aria-label="编辑事项"]').trigger('click')
     expect(body.get('[aria-label="记录说明"]').element.value).toBe(task.details)
     await body.get('button[aria-label="关联发版，已选 1 个版本"]').trigger('click')
     const options = body.findAll('[role="option"]')
     expect(options.map(option => option.attributes('aria-selected'))).toEqual(['true', 'false'])
     await options[1].trigger('click')
     await body.get('form[id^="lifecycle-"]').trigger('submit'); await flushPromises()
     expect(mocks.save).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ id: 8, version_ids: [3, 5], details: task.details }))
   } finally {
     page.unmount()
   }
 })
 it('uses backend pagination with default ten rows and supports changing pages', async () => {
   mocks.page.mockImplementation(async (_id, _kind, query) => ({ items: [{ id: Number(query.offset) + 1, title: 'task', status: 'todo', kind: 'action', priority: 'normal' }], total: 25 }))
   const page = mountPage('tasks'); await flushPromises()
   expect(mocks.page).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ limit: 10, offset: 0 }), expect.anything())
   await page.get('[aria-label="下一页"]').trigger('click'); await flushPromises()
   expect(mocks.page).toHaveBeenLastCalledWith(7, 'tasks', expect.objectContaining({ limit: 10, offset: 10 }), expect.anything())
   page.unmount()
 })
 it('deletes a task only after confirmation and refreshes the list', async () => {
   mocks.page.mockResolvedValue({ items: [{ id: 8, title: '删除验证', kind: 'action', priority: 'normal', status: 'todo' }], total: 1 })
   const page = mountPage('tasks'); await flushPromises()
   await page.get('[aria-label="删除事项"]').trigger('click')
   expect(page.text()).toContain('确认删除事项“删除验证”？')
   await page.get('[data-testid="confirm-dialog-cancel"]').trigger('click')
   expect(mocks.remove).not.toHaveBeenCalled()

   await page.get('[aria-label="删除事项"]').trigger('click')
   mocks.page.mockResolvedValueOnce({ items: [], total: 0 })
   await page.get('[data-testid="confirm-dialog-confirm"]').trigger('click'); await flushPromises()
   expect(mocks.remove).toHaveBeenCalledWith(7, 8)
   expect(page.find('[aria-label="删除事项"]').exists()).toBe(false)
   expect(page.emitted('changed')).toHaveLength(1)
   page.unmount()
 })
 it('uses a searchable multi-select and preserves selections across results', async () => {
   mocks.page.mockImplementation(async (_id, kind, query) => {
     if (kind !== 'tasks') return { items: [], total: 0 }
     const id = query.keyword ? 12 : query.offset ? 11 : 10
     return { items: [{ id, title: query.keyword || '原事项', kind: 'action', status: 'done' }], total: query.keyword ? 1 : 2 }
   })
   const record = { id: 8, name: '1.0', status: 'releasing', task_ids: [10, 11] }
   const editor = mount(LifecycleEditor, { props: { open: true, kind: 'versions', productId: 7, record }, attachTo: document.body })
   await flushPromises()
   const body = new DOMWrapper(document.body)
   expect(body.find('[aria-label="搜索可选事项"]').exists()).toBe(false)
   const trigger = body.get('button[aria-label="纳入版本的事项，已选 2 项"]')
   await trigger.trigger('click'); await flushPromises()
   const list = body.get('[aria-multiselectable="true"]')
   expect(list.get('[role="option"]').attributes('aria-selected')).toBe('true')
   await list.get('[role="option"]').trigger('click')
   Object.defineProperties(list.element, {
     scrollTop: { value: 250, writable: true },
     scrollHeight: { value: 450 },
     clientHeight: { value: 200 },
   })
   list.element.dispatchEvent(new Event('scroll'))
   await flushPromises()
   expect(mocks.page).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ offset: 1 }), expect.anything())
   expect(list.findAll('[role="option"]').map(option => option.attributes('aria-selected'))).toEqual(['false', 'true'])
   expect(body.find('[aria-label="可选事项分页"]').exists()).toBe(false)
   await body.get('[aria-label="搜索可选事项"]').setValue('新事项'); await flushPromises()
   expect(mocks.page).toHaveBeenCalledWith(7, 'tasks', expect.objectContaining({ assignable: true, version_id: 8, keyword: '新事项' }), expect.anything())
   await list.get('[role="option"]').trigger('click')
   await body.get('[aria-label="搜索可选事项"]').trigger('keydown', { key: 'Escape' })
   expect(body.find('[aria-multiselectable="true"]').exists()).toBe(false)
   await body.get('form').trigger('submit')
   expect(editor.emitted('save')[0][0].task_ids).toEqual([11, 12])
   expect(record.task_ids).toEqual([10, 11])
   editor.unmount()
 })
 it('records a channel submission before its formal version exists', async () => {
   mocks.options.mockResolvedValue({ versions: [] })
   const page = mountPage('distributions'); await flushPromises()
   await page.findAll('button').find(button => button.text() === '新增发行记录').trigger('click')
   const editor = page.getComponent(LifecycleEditor)
   await editor.get('select[aria-label="发行渠道"]').setValue('9')
   await editor.get('input[placeholder="例如 2.8.1"]').setValue('2.8.1-rc.1')
   await editor.get('form').trigger('submit'); await flushPromises()
   expect(mocks.save).toHaveBeenCalledWith(7, 'distributions', expect.objectContaining({ version_id: 0, platform_version: '2.8.1-rc.1' }))
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
   const record = reactive({ id: 1, name: '1.0', status: 'testing', deployment_ids: [99], platform_release_ids: [42] })
   const page = mount(LifecycleEditor, { props: { open: true, kind: 'versions', productId: 7, record }, global: { stubs: { Teleport: true } } }); await flushPromises()
   await page.get('input[required]').setValue('1.1')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[0][0]).toMatchObject({ name: '1.1', deployment_ids: [99], platform_release_ids: [42] })
   expect(record.name).toBe('1.0')
   page.unmount()
 })
})


describe('distribution audience', () => {
 it('offers only the channel release unit when linking a formal version', async () => {
   const channels = [{ id: 9, name: 'Google Play', release_unit_id: 1 }]
   const releaseUnits = [{ id: 1, name: 'Android' }, { id: 2, name: '桌面端' }]
   const versions = [{ id: 3, name: '1.0', release_unit_id: 1 }, { id: 4, name: '1.0', release_unit_id: 2 }]
   const record = { id: 8, channel_id: 9, version_id: 0, platform_version: '1.0', status: 'in_review' }
   const page = mount(LifecycleEditor, {
     props: { open: true, kind: 'distributions', productId: 7, channels, releaseUnits, versions, record },
     global: { stubs: { Teleport: true } },
   })
   const select = page.get('select[aria-label="关联产品版本"]')
   expect(select.findAll('option').map(option => option.text())).toEqual(['暂不关联', 'Android · 1.0'])
   await select.setValue('3')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[0][0]).toMatchObject({ id: 8, channel_id: 9, version_id: 3, status: 'in_review' })
   page.unmount()
 })
 it('defaults new distributions to testing and preserves unclassified history when editing', async () => {
   const page = mount(LifecycleEditor, { props: { open: true, kind: 'distributions', productId: 7, channels: product.channels, versions: [{ id: 3, name: '1.0' }] }, global: { stubs: { Teleport: true } } })
   await flushPromises()
   expect(page.get('select[aria-label="发行分类"]').element.value).toBe('testing')
   await page.get('select[aria-label="发行渠道"]').setValue('9')
   await page.get('select[aria-label="关联产品版本"]').setValue('3')
   const toggle = page.findAll('button').find(button => button.text() === '渠道版本与构建标识')
   expect(toggle.attributes('aria-expanded')).toBe('false')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[0][0]).toMatchObject({ audience: 'testing', version_id: 3, channel_id: 9, platform_version: '' })
   await toggle.trigger('click')
   await page.get('input[placeholder="1.0"]').setValue('1.0-beta')
   await page.get('form').trigger('submit')
   expect(page.emitted('save')[1][0]).toMatchObject({ version_id: 3, platform_version: '1.0-beta' })
   await page.setProps({ open: false })
   await page.setProps({ open: true, record: { id: 4, audience: 'unclassified', version_id: 3, channel_id: 9, platform_version: 'old', status: 'published' } })
   expect(page.get('select[aria-label="发行分类"]').element.value).toBe('unclassified')
   expect(page.get('input[placeholder="1.0"]').element.value).toBe('old')
   page.unmount()
 })
 it('filters distributions by audience and resets it with the other filters', async () => {
   const page = mountPage('distributions')
   await flushPromises()
   await page.get('select[aria-label="发行分类筛选"]').setValue('production')
   await flushPromises()
   expect(mocks.page).toHaveBeenLastCalledWith(7, 'distributions', expect.objectContaining({ audience: 'production', offset: 0 }), expect.anything())
   await page.get('[aria-label="重置筛选"]').trigger('click')
   await flushPromises()
   expect(mocks.page).toHaveBeenLastCalledWith(7, 'distributions', expect.objectContaining({ audience: '' }), expect.anything())
   page.unmount()
 })
})
