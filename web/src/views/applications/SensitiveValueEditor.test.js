import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import SensitiveValueEditor from './SensitiveValueEditor.vue'
import { inspectDeploymentTemplateSecret } from '../../api/applications.js'

vi.mock('../../api/applications.js', () => ({ inspectDeploymentTemplateSecret: vi.fn() }))
function editor() {
  const item = reactive({ key: 'MYSQL_DSN', original_key: 'MYSQL_DSN', value: '' })
  return { item, wrapper: mount(SensitiveValueEditor, { props: { item, applicationId: '1', templateId: 4, revision: 2 } }) }
}
const reveal = wrapper => wrapper.get('button').trigger('click')
beforeEach(() => inspectDeploymentTemplateSecret.mockReset().mockResolvedValue({ key: 'MYSQL_DSN', revision: 2, value: 'saved-secret\nsecond-line' }))

describe('SensitiveValueEditor', () => {
  it('reveals on demand without adding a replacement to the save payload, and clears on hide', async () => {
    const { item, wrapper } = editor()
    expect(inspectDeploymentTemplateSecret).not.toHaveBeenCalled()
    expect(wrapper.get('input').attributes('type')).toBe('password')
    await reveal(wrapper)
    await flushPromises()
    expect(inspectDeploymentTemplateSecret).toHaveBeenCalledWith('1', 4, { key: 'MYSQL_DSN', revision: 2 }, expect.objectContaining({ signal: expect.any(AbortSignal), cache: 'no-store' }))
    expect(wrapper.get('textarea').element.value).toBe('saved-secret\nsecond-line')
    expect(item.value).toBe('')
    await reveal(wrapper)
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.get('input').element.value).toBe('')
    await reveal(wrapper)
    await flushPromises()
    expect(inspectDeploymentTemplateSecret).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(item.value).toBe('')
  })

  it('preserves multiline edits across hide and reveal without fetching over a draft', async () => {
    const { item, wrapper } = editor()
    await reveal(wrapper)
    await flushPromises()
    await wrapper.get('textarea').setValue('new-secret\nsecond-line')
    await reveal(wrapper)
    expect(wrapper.get('input').attributes('type')).toBe('password')
    expect(item.value).toBe('new-secret\nsecond-line')
    await reveal(wrapper)
    expect(wrapper.get('textarea').element.value).toBe('new-secret\nsecond-line')
    expect(inspectDeploymentTemplateSecret).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('allows replacement without reading the saved value', async () => {
    const { item, wrapper } = editor()
    await wrapper.get('input').setValue('replacement')
    await reveal(wrapper)
    expect(wrapper.get('textarea').element.value).toBe('replacement')
    expect(item.value).toBe('replacement')
    expect(inspectDeploymentTemplateSecret).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each(['revision', 'item', 'unmount'])('ignores a late response after %s changes', async change => {
    let finish
    inspectDeploymentTemplateSecret.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const { item, wrapper } = editor()
    await reveal(wrapper)
    const signal = inspectDeploymentTemplateSecret.mock.calls[0][3].signal
    if (change === 'revision') await wrapper.setProps({ revision: 3 })
    else if (change === 'item') await wrapper.setProps({ item: reactive({ key: 'OTHER', original_key: 'OTHER', value: '' }) })
    else wrapper.unmount()
    expect(signal.aborted).toBe(true)
    finish({ key: 'MYSQL_DSN', revision: 2, value: 'stale-secret' })
    await flushPromises()
    expect(item.value).toBe('')
    expect(wrapper.emitted('error')).toBeUndefined()
    if (change !== 'unmount') { expect(wrapper.find('textarea').exists()).toBe(false); wrapper.unmount() }
  })

  it('does not overwrite a replacement entered while the saved value is loading', async () => {
    let finish
    inspectDeploymentTemplateSecret.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const { item, wrapper } = editor()
    await reveal(wrapper)
    await wrapper.get('input').setValue('new-draft')
    finish({ key: 'MYSQL_DSN', revision: 2, value: 'old-secret' })
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('new-draft')
    expect(item.value).toBe('new-draft')
    wrapper.unmount()
  })

  it('rejects a mismatched response and reports a shared error event', async () => {
    inspectDeploymentTemplateSecret.mockResolvedValue({ key: 'OTHER', revision: 2, value: 'wrong-secret' })
    const { item, wrapper } = editor()
    await reveal(wrapper)
    await flushPromises()
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(item.value).toBe('')
    expect(wrapper.emitted('error')[0][0]).toContain('配置已改变')
    wrapper.unmount()
  })
})
