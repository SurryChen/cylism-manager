import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import SensitiveFileEditor from './SensitiveFileEditor.vue'
import { inspectDeploymentTemplateFile } from '../../api/applications.js'

vi.mock('../../api/applications.js', () => ({ inspectDeploymentTemplateFile: vi.fn() }))

const fieldResponse = () => ({ revision: 2, supported: true, fields: [
  { line: 1, name: 'MYSQL_HOST' },
  { line: 2, name: 'MYSQL_PASSWORD' },
] })
function editor(key = 'env') {
  const item = reactive({ key, original_key: key, value: '' })
  const wrapper = mount(SensitiveFileEditor, { props: { item, applicationId: '1', templateId: 4, revision: 2 }, global: { stubs: { Teleport: true } } })
  return { item, wrapper }
}
const button = (wrapper, text) => wrapper.findAll('button').find(button => button.text() === text)

describe('SensitiveFileEditor', () => {
  it('filters long field lists without losing pending changes and keeps new fields visible', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue({ revision: 2, supported: true, fields: Array.from({ length: 12 }, (_, line) => ({ line, name: `SETTING_${line}` })) })
    const { item, wrapper } = editor()
    await flushPromises()
    const input = wrapper.find('[aria-label="配置项 SETTING_0 的值"]')
    expect(input.attributes('readonly')).toBeDefined()
    await input.trigger('focus')
    expect(input.attributes('readonly')).toBeUndefined()
    await input.setValue('pending')
    await input.trigger('blur')
    await wrapper.find('[aria-label="搜索配置项"]').setValue('setting_11')
    expect(wrapper.findAll('.env-file-field')).toHaveLength(1)
    expect(item.file_edit.changes).toEqual([{ line: 0, name: 'SETTING_0', value: 'pending' }])
    await button(wrapper, '添加配置项').trigger('click')
    expect(wrapper.find('[aria-label="搜索配置项"]').element.value).toBe('')
    expect(wrapper.find('[aria-label="新增配置项名称"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="配置项 SETTING_0 的值"]').element.value).toBe('pending')
    expect(inspectDeploymentTemplateFile).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('hides every Secret field and submits only changed assignments', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue(fieldResponse())
    const { item, wrapper } = editor()
    await flushPromises()
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').element.value).toBe('')
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').attributes('type')).toBe('password')
    expect(wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').element.value).toBe('')
    expect(wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').attributes('type')).toBe('password')
    await wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').setValue('new.svc')
    expect(item.value).toBe('')
    expect(item.file_edit).toEqual({ key: 'env', changes: [{ line: 1, name: 'MYSQL_HOST', value: 'new.svc' }] })
    expect(button(wrapper, '查看并编辑全文').element.disabled).toBe(true)
    await button(wrapper, '重新读取').trigger('click')
    expect(inspectDeploymentTemplateFile).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('error').at(-1)).toEqual(['请先保存或撤销修改，再重新读取'])
    wrapper.unmount()
  })

  it('fetches and reveals only the clicked field, then clears an unchanged value when hidden', async () => {
    inspectDeploymentTemplateFile.mockReset().mockImplementation((_application, _template, request) => Promise.resolve(request.mode === 'fields'
      ? fieldResponse() : { revision: 2, line: 1, name: 'MYSQL_HOST', value: 'mysql.svc' }))
    const { item, wrapper } = editor()
    await flushPromises()
    await wrapper.find('[aria-label="查看配置项 MYSQL_HOST 的值"]').trigger('click')
    await flushPromises()
    expect(inspectDeploymentTemplateFile.mock.calls[1][2]).toEqual({ key: 'env', revision: 2, mode: 'field', line: 1, name: 'MYSQL_HOST' })
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').element.value).toBe('mysql.svc')
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').attributes('type')).toBe('text')
    expect(wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').element.value).toBe('')
    expect(item.file_edit).toBeUndefined()
    await wrapper.find('[aria-label="隐藏配置项 MYSQL_HOST 的值"]').trigger('click')
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').element.value).toBe('')
    expect(wrapper.html()).not.toContain('mysql.svc')
    wrapper.unmount()
  })

  it('keeps edited drafts masked on hide and never replaces them with saved values', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue(fieldResponse())
    const { item, wrapper } = editor()
    await flushPromises()
    await wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').setValue('new-password')
    await wrapper.find('[aria-label="查看配置项 MYSQL_PASSWORD 的值"]').trigger('click')
    expect(wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').attributes('type')).toBe('text')
    await wrapper.find('[aria-label="隐藏配置项 MYSQL_PASSWORD 的值"]').trigger('click')
    expect(wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').attributes('type')).toBe('password')
    expect(item.file_edit.changes).toEqual([{ line: 2, name: 'MYSQL_PASSWORD', value: 'new-password' }])
    expect(inspectDeploymentTemplateFile).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('keeps values hidden when a field read fails or the template revision changes', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValueOnce(fieldResponse()).mockRejectedValueOnce(new Error('无法记录审计'))
    const { item, wrapper } = editor()
    await flushPromises()
    await wrapper.find('[aria-label="查看配置项 MYSQL_HOST 的值"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('error').at(-1)).toEqual(['无法记录审计'])
    inspectDeploymentTemplateFile.mockResolvedValueOnce({ revision: 3, line: 1, name: 'MYSQL_HOST', value: 'stale.svc' })
    await wrapper.find('[aria-label="查看配置项 MYSQL_HOST 的值"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').element.value).toBe('')
    expect(wrapper.emitted('error').at(-1)[0]).toContain('模板版本已改变')
    expect(item.file_edit).toBeUndefined()
    wrapper.unmount()
  })

  it('restores pending edits when a shared mounted file editor is remounted', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue(fieldResponse())
    const { item, wrapper } = editor()
    await flushPromises()
    await wrapper.find('[aria-label="配置项 MYSQL_HOST 的值"]').setValue('pending.svc')
    wrapper.unmount()
    const restored = mount(SensitiveFileEditor, { props: { item, applicationId: '1', templateId: 4, revision: 2 } })
    await flushPromises()
    expect(restored.find('[aria-label="配置项 MYSQL_HOST 的值"]').element.value).toBe('pending.svc')
    expect(item.file_edit.changes).toEqual([{ line: 1, name: 'MYSQL_HOST', value: 'pending.svc' }])
    restored.unmount()
  })

  it('supports explicit empty values, removal, restoration and new assignments', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue(fieldResponse())
    const { item, wrapper } = editor()
    await flushPromises()
    await wrapper.find('[aria-label="配置项 MYSQL_PASSWORD 的值"]').setValue('')
    expect(item.file_edit.changes).toEqual([{ line: 2, name: 'MYSQL_PASSWORD', value: '' }])
    await wrapper.find('[aria-label="移除配置项 MYSQL_PASSWORD"]').trigger('click')
    expect(item.file_edit.changes).toEqual([{ line: 2, name: 'MYSQL_PASSWORD', remove: true }])
    await wrapper.find('[aria-label="恢复配置项 MYSQL_PASSWORD"]').trigger('click')
    await button(wrapper, '添加配置项').trigger('click')
    await wrapper.find('[aria-label="新增配置项名称"]').setValue('LOG_LEVEL')
    await wrapper.find('[aria-label="配置项 LOG_LEVEL 的值"]').setValue('debug')
    expect(item.file_edit.changes).toContainEqual({ line: -1, name: 'LOG_LEVEL', value: 'debug' })
    wrapper.unmount()
  })

  it('loads full contents only after explicit confirmation and clears an unchanged reveal on hide', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue({ revision: 2, content: 'PRIVATE KEY CONTENT' })
    const { item, wrapper } = editor('signing.pem')
    await flushPromises()
    expect(inspectDeploymentTemplateFile).not.toHaveBeenCalled()
    await button(wrapper, '查看并编辑全文').trigger('click')
    expect(inspectDeploymentTemplateFile).not.toHaveBeenCalled()
    await button(wrapper, '查看并编辑').trigger('click')
    await flushPromises()
    expect(inspectDeploymentTemplateFile.mock.calls[0].slice(0, 3)).toEqual(['1', 4, { key: 'signing.pem', revision: 2, mode: 'content' }])
    expect(wrapper.find('[aria-label="文件内容"]').element.value).toBe('PRIVATE KEY CONTENT')
    await button(wrapper, '隐藏全文').trigger('click')
    expect(item.value).toBe('')
    expect(wrapper.html()).not.toContain('PRIVATE KEY CONTENT')
    wrapper.unmount()
  })

  it('preserves an edited hidden draft until save or explicit undo', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue({ revision: 2, content: 'old' })
    const { item, wrapper } = editor('config.yaml')
    await button(wrapper, '查看并编辑全文').trigger('click')
    await button(wrapper, '查看并编辑').trigger('click')
    await flushPromises()
    await wrapper.find('[aria-label="文件内容"]').setValue('new')
    await button(wrapper, '隐藏全文').trigger('click')
    expect(item.value).toBe('new')
    expect(wrapper.find('[aria-label="文件内容"]').exists()).toBe(false)
    await button(wrapper, '撤销修改').trigger('click')
    expect(item.value).toBe('')
    wrapper.unmount()
  })

  it('falls back for unsupported env syntax and reports read failures', async () => {
    inspectDeploymentTemplateFile.mockReset().mockResolvedValue({ revision: 2, supported: false, message: '包含多行值，请使用全文编辑' })
    const { wrapper } = editor()
    await flushPromises()
    expect(wrapper.text()).toContain('包含多行值')
    expect(wrapper.findAll('.env-file-field')).toHaveLength(0)
    inspectDeploymentTemplateFile.mockRejectedValueOnce(new Error('审计写入失败'))
    await button(wrapper, '逐项编辑').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('error').at(-1)).toEqual(['审计写入失败'])
    wrapper.unmount()
  })

  it('ignores late responses after closing and aborts the request', async () => {
    let resolve
    inspectDeploymentTemplateFile.mockReset().mockReturnValue(new Promise(done => { resolve = done }))
    const { item, wrapper } = editor()
    const signal = inspectDeploymentTemplateFile.mock.calls[0][3].signal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve(fieldResponse())
    await flushPromises()
    expect(item.value).toBe('')
    expect(item.file_edit).toBeUndefined()
  })
})
