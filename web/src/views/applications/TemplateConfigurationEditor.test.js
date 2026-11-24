import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { getUnusedTemplateSecretKeys } from '../../api/applications.js'
import TemplateConfigurationEditor from './TemplateConfigurationEditor.vue'

vi.mock('../../api/applications.js', () => ({
  getUnusedTemplateSecretKeys: vi.fn(),
}))

function editor(overrides = {}, props = {}) {
  const form = reactive({ config_items: [], secret_items: [], file_mounts: [], delete_secret_keys: [], ...overrides })
  const wrapper = mount(TemplateConfigurationEditor, { props: { form, ...props } })
  return { form, wrapper }
}
const addFile = wrapper => wrapper.findAll('button').find(button => button.text() === '添加文件').trigger('click')
const clickButton = (wrapper, label) => wrapper.findAll('button').find(button => button.text() === label).trigger('click')
const savedSecret = (key, file_only = true) => ({ key, original_key: key, value: '', enabled: true, file_only })
const secretMount = (key, mount_path) => ({ source_type: 'application_secret', key, mount_path })

async function removeFirstMount(wrapper) {
  await wrapper.find('.file-settings-button').trigger('click')
  await wrapper.find('[aria-label="移除配置文件"]').trigger('click')
}

describe('TemplateConfigurationEditor', () => {
  it('keeps file settings out of the default view and opens only one file at a time', async () => {
    const { wrapper } = editor({ config_items: [{ key: 'a.yaml', value: 'port: 80', file_only: true }, { key: 'b.yaml', value: 'port: 81', file_only: true }], file_mounts: ['a.yaml', 'b.yaml'].map(key => ({ source_type: 'application_config', key, mount_path: `/etc/app/${key}` })) })
    const files = wrapper.findAll('.configuration-file')
    expect(files.every(file => !file.element.open)).toBe(true)
    expect(wrapper.find('[aria-label="文件名称"]').exists()).toBe(false)
    await files[0].find('summary').trigger('click')
    expect(files[0].element.open).toBe(true)
    await files[0].find('[aria-label="文件内容"]').setValue('port: 90')
    await files[1].find('summary').trigger('click')
    expect(files[0].element.open).toBe(false)
    expect(files[1].element.open).toBe(true)
    await files[0].find('.file-settings-button').trigger('click')
    expect(files[0].element.open).toBe(true)
    expect(files[1].element.open).toBe(false)
    expect(files[0].find('[aria-label="文件名称"]').element.value).toBe('a.yaml')
    expect(files[0].find('[aria-label="文件内容"]').element.value).toBe('port: 90')
    wrapper.unmount()
  })

  it('keeps multiple new files distinct and removes unused inline contents', async () => {
    const { form, wrapper } = editor()
    await addFile(wrapper)
    await addFile(wrapper)
    const files = wrapper.findAll('.configuration-file')
    await files[1].find('[aria-label="文件名称"]').setValue('second.yaml')
    await files[1].find('[aria-label="文件内容"]').setValue('second')
    await files[0].find('.file-settings-button').trigger('click')
    await files[0].find('[aria-label="文件名称"]').setValue('first.yaml')
    await files[0].find('[aria-label="文件内容"]').setValue('first')
    expect(form.config_items.map(item => [item.key, item.value, item.file_only])).toEqual([['first.yaml', 'first', true], ['second.yaml', 'second', true]])
    expect(wrapper.findAll('.environment-entry')).toHaveLength(0)
    await files[0].find('[aria-label="移除配置文件"]').trigger('click')
    expect(form.config_items.map(item => item.key)).toEqual(['second.yaml'])
    expect(form.file_mounts.map(item => item.key)).toEqual(['second.yaml'])
    wrapper.unmount()
  })

  it('moves an inline file into sensitive storage without losing its content or path', async () => {
    const { form, wrapper } = editor()
    await addFile(wrapper)
    await wrapper.find('[aria-label="文件名称"]').setValue('env')
    await wrapper.find('[aria-label="文件内容"]').setValue('MYSQL_DSN=example')
    await wrapper.find('[aria-label="文件挂载路径"]').setValue('/app/secrets/env')
    await wrapper.find('[aria-label="文件来源类型"].select-menu-native').setValue('application_secret')
    expect(form.config_items).toEqual([])
    expect(form.secret_items).toMatchObject([{ key: 'env', value: 'MYSQL_DSN=example', file_only: true }])
    expect(form.file_mounts).toMatchObject([{ source_type: 'application_secret', key: 'env', mount_path: '/app/secrets/env' }])
    await wrapper.find('[aria-label="同时注入环境变量"]').setValue(true)
    expect(form.secret_items[0].file_only).toBe(false)
    wrapper.unmount()
  })

  it('preserves saved sensitive contents and legacy environment injection', async () => {
    const { form, wrapper } = editor({ secret_items: [{ key: 'env', value: '', original_key: 'env', enabled: true, file_only: false }], file_mounts: [{ source_type: 'application_secret', key: 'env', source_name: '', mount_path: '/app/secrets/env' }] })
    expect(wrapper.find('[aria-label="文件内容"]').attributes('placeholder')).toContain('留空保留原值')
    await wrapper.find('.file-settings-button').trigger('click')
    expect(wrapper.find('[aria-label="同时注入环境变量"]').element.checked).toBe(true)
    await wrapper.find('[aria-label="文件来源类型"].select-menu-native').setValue('application_config')
    expect(wrapper.emitted('error')[0][0]).toContain('重新填写内容')
    expect(form.file_mounts[0].source_type).toBe('application_secret')
    expect(form.secret_items[0].original_key).toBe('env')
    wrapper.unmount()
  })

  it('renames shared file keys together and retains legacy variables when unmounting', async () => {
    const { form, wrapper } = editor({ config_items: [{ key: 'config.yaml', value: 'port: 80', enabled: true, file_only: false }], file_mounts: ['/first/config.yaml', '/second/config.yaml'].map(mount_path => ({ source_type: 'application_config', key: 'config.yaml', mount_path })) })
    await wrapper.find('.file-settings-button').trigger('click')
    await wrapper.find('[aria-label="文件名称"]').setValue('app.yaml')
    expect(form.file_mounts.map(file => file.key)).toEqual(['app.yaml', 'app.yaml'])
    await wrapper.find('[aria-label="移除配置文件"]').trigger('click')
    await wrapper.find('.file-settings-button').trigger('click')
    await wrapper.find('[aria-label="移除配置文件"]').trigger('click')
    expect(form.config_items).toMatchObject([{ key: 'app.yaml', value: 'port: 80', file_only: false }])
    expect(wrapper.findAll('.environment-entry')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not overwrite another entry when changing storage', async () => {
    const { form, wrapper } = editor({ secret_items: [{ key: 'env', value: '', original_key: 'env', enabled: true }] })
    await addFile(wrapper)
    await wrapper.find('[aria-label="文件名称"]').setValue('env')
    await wrapper.find('[aria-label="文件来源类型"].select-menu-native').setValue('application_secret')
    expect(wrapper.emitted('error')[0][0]).toContain('同名配置')
    expect(form.file_mounts[0].source_type).toBe('application_config')
    expect(form.secret_items).toHaveLength(1)
    wrapper.unmount()
  })

  it.each(['保留值', '删除值'])('confirms the last reference with %s', async choice => {
    const key = 'client-secret'
    const { form, wrapper } = editor({
      secret_items: [savedSecret(key), savedSecret('MYSQL_DSN', false)],
      file_mounts: [secretMount(key, '/a/key'), secretMount(key, '/b/key')],
    })
    await removeFirstMount(wrapper)
    expect(form.file_mounts.map(file => file.mount_path)).toEqual(['/b/key'])
    expect(form.delete_secret_keys).toEqual([])
    expect(wrapper.text()).not.toContain('移除敏感配置')

    await removeFirstMount(wrapper)
    expect(wrapper.text()).toContain('最后一个引用')
    await clickButton(wrapper, '取消')
    expect(form.file_mounts).toHaveLength(1)
    await removeFirstMount(wrapper)
    await clickButton(wrapper, choice)
    expect(form.file_mounts).toEqual([])
    expect(form.delete_secret_keys).toEqual(choice === '删除值' ? [key] : [])
    expect(form.secret_items.map(item => item.key)).toEqual(choice === '删除值' ? ['MYSQL_DSN'] : [key, 'MYSQL_DSN'])
    expect(form.secret_items.find(item => item.key === 'MYSQL_DSN').value).toBe('')
    expect(wrapper.findAll('.environment-entry')).toHaveLength(1)
    wrapper.unmount()
  })

  it('preserves the environment reference when removing the only mount', async () => {
    const { form, wrapper } = editor({
      secret_items: [savedSecret('credential', false)],
      file_mounts: [secretMount('credential', '/app/secret')],
    })
    await removeFirstMount(wrapper)
    expect(form.file_mounts).toEqual([])
    expect(form.secret_items).toEqual([savedSecret('credential', false)])
    expect(form.delete_secret_keys).toEqual([])
    expect(wrapper.findAll('.environment-entry')).toHaveLength(1)
    wrapper.unmount()
  })

  it('queues and deselects historical keys without editing valid saved values', async () => {
    getUnusedTemplateSecretKeys.mockResolvedValue({ revision: 3, unused_secret_keys: ['old-file', 'old-token', 'reused-key'] })
    const { form, wrapper } = editor({
      secret_items: [savedSecret('MYSQL_DSN', false), savedSecret('reused-key', false)],
    }, { applicationId: 1, templateId: 2, revision: 3 })

    await clickButton(wrapper, '清理无引用的敏感配置')
    await flushPromises()
    expect(getUnusedTemplateSecretKeys).toHaveBeenCalledWith(1, 2)
    expect(wrapper.findAll('input[type=checkbox][value]').map(input => input.element.value)).toEqual(['old-file', 'old-token'])
    await wrapper.find('input[value="old-token"]').setValue(true)
    await clickButton(wrapper, '确认清理选项')
    expect(form.delete_secret_keys).toEqual(['old-token'])
    expect(form.secret_items).toEqual([savedSecret('MYSQL_DSN', false), savedSecret('reused-key', false)])

    await clickButton(wrapper, '清理无引用的敏感配置')
    await flushPromises()
    expect(wrapper.find('input[value="old-token"]').element.checked).toBe(true)
    await wrapper.find('input[value="old-token"]').setValue(false)
    await clickButton(wrapper, '确认清理选项')
    expect(form.delete_secret_keys).toEqual([])
    wrapper.unmount()
  })

  it('rejects cleanup selections from a newer revision', async () => {
    getUnusedTemplateSecretKeys.mockResolvedValue({ revision: 4, unused_secret_keys: ['old-key'] })
    const { form, wrapper } = editor({}, { applicationId: 1, templateId: 2, revision: 3 })
    await clickButton(wrapper, '清理无引用的敏感配置')
    await flushPromises()
    expect(wrapper.emitted('error')?.[0]).toEqual(['模板已更新，请重新打开后清理'])
    expect(wrapper.find('input[value="old-key"]').exists()).toBe(false)
    expect(form.delete_secret_keys).toEqual([])
    wrapper.unmount()
  })
})
