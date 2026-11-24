import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import TemplateConfigurationEditor from './TemplateConfigurationEditor.vue'

function editor(overrides = {}) {
  const form = reactive({ config_items: [], secret_items: [], file_mounts: [], ...overrides })
  const wrapper = mount(TemplateConfigurationEditor, { props: { form } })
  return { form, wrapper }
}
const addFile = wrapper => wrapper.findAll('button').find(button => button.text() === '添加文件').trigger('click')

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
})
