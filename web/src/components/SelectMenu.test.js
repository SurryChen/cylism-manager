import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SelectMenu from './SelectMenu.vue'

describe('SelectMenu', () => {
  it('opens, selects an option, and emits a model update', async () => {
    const wrapper = mount(SelectMenu, { props: { modelValue: '', placeholder: '全部结果', options: [{ value: 'ok', label: '成功' }] } })
    await wrapper.get('.select-menu-trigger').trigger('click')
    await wrapper.get('.select-menu-option').trigger('click')
    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['ok'])
    expect(wrapper.emitted('change')[0]).toEqual(['ok'])
    expect(wrapper.find('.select-menu-options').exists()).toBe(false)
  })
})
