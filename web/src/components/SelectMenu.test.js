import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
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

  it('preserves numeric values when changed through the native form control', async () => {
    const wrapper = mount(SelectMenu, { props: { modelValue: 0, options: [{ value: 0, label: '全部' }, { value: 500, label: '500 条' }] } })

    await wrapper.get('.select-menu-native').setValue('500')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual([500])
    expect(wrapper.emitted('change')[0]).toEqual([500])
  })

  it('preserves required, disabled, id, and test-hook form contracts', async () => {
    const requiredWrapper = mount(SelectMenu, { props: { modelValue: '', options: [{ value: 'nanobot', label: 'Nanobot' }], required: true } })
    expect(requiredWrapper.get('.select-menu-native').element.checkValidity()).toBe(false)

    const wrapper = mount(SelectMenu, {
      attrs: { 'data-testid': 'runtime-selector' },
      props: { id: 'runtime-selector', modelValue: '', options: [{ value: 'nanobot', label: 'Nanobot' }], required: true, disabled: true },
    })
    const native = wrapper.get('.select-menu-native')

    expect(native.attributes('data-testid')).toBe('runtime-selector')
    expect(wrapper.get('.select-menu-trigger').attributes('id')).toBe('runtime-selector')
    expect(wrapper.get('.select-menu-trigger').attributes('disabled')).toBeDefined()
    expect(native.attributes('required')).toBeDefined()
    expect(native.attributes('disabled')).toBeDefined()

    await wrapper.get('.select-menu-trigger').trigger('click')
    expect(wrapper.find('.select-menu-options').exists()).toBe(false)
  })

  it('uses option slots without converting their selection semantics', async () => {
    const Harness = defineComponent({
      components: { SelectMenu },
      template: '<SelectMenu v-model="value"><option value="TCP">TCP</option><option value="UDP">UDP</option></SelectMenu>',
      setup: () => ({ value: ref('TCP') }),
    })
    const wrapper = mount(Harness)

    await wrapper.get('.select-menu-native').setValue('UDP')

    expect(wrapper.vm.value).toBe('UDP')
  })

  it('renders structured option descriptions while allowing a separate trigger label', async () => {
    const wrapper = mount(SelectMenu, {
      props: {
        modelValue: 5,
        options: [{ value: 5, label: '线上环境', triggerLabel: '线上环境 · project-freedom-proxy', description: 'project-freedom-proxy' }],
      },
    })

    expect(wrapper.get('.select-menu-trigger').text()).toContain('线上环境 · project-freedom-proxy')
    await wrapper.get('.select-menu-trigger').trigger('click')
    expect(wrapper.get('.select-menu-option-label').text()).toBe('线上环境')
    expect(wrapper.get('.select-menu-option-description').text()).toBe('project-freedom-proxy')
  })
})
