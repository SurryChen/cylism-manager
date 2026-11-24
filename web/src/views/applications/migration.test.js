import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { migrationForm, migrationPayload, releasePayload } from './migration.js'
import MigrationConfigurationEditor from './MigrationConfigurationEditor.vue'

describe('migration configuration', () => {
 it('saves configuration without persisting a release target', () => {
  const form = migrationForm({ enabled: true, command: ['migrate', '--up'], target: '123' })
  expect(migrationPayload(form)).toEqual({ enabled: true, command: ['migrate', '--up'], directory: '/app/migrations', database_prefix: 'DATABASE', timeout_seconds: 600 })
  expect(migrationPayload(migrationForm())).toBeUndefined()
 })
 it('only sends a target for enabled templates and preserves large versions as strings', () => {
  const form = { template_id: 2, version: '1.0', migration_target: '20261122000112345678901234567890' }
  expect(releasePayload(form, { spec: { migration: { enabled: true } } }).migration.target).toBe(form.migration_target)
  expect(releasePayload(form, { spec: {} })).toEqual({ template_id: 2, version: '1.0' })
 })
 it('reveals editable configuration only when enabled', async () => {
  const form = reactive(migrationForm())
  const wrapper = mount(MigrationConfigurationEditor, { props: { form } })
  expect(wrapper.find('[aria-label="迁移命令"]').exists()).toBe(false)
  await wrapper.get('input[type="checkbox"]').setValue(true)
  await wrapper.get('[aria-label="迁移命令"]').setValue('migrate\n--up')
  expect(migrationPayload(form).command).toEqual(['migrate', '--up'])
  expect(wrapper.text()).toContain('应用回滚不会撤销 SQL')
  wrapper.unmount()
 })
})
