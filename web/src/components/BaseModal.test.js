import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseModal from './BaseModal.vue'

const mountModal = props => mount(BaseModal, {
  props,
  slots: {
    default: '<p data-testid="body">内容</p>',
    actions: '<button data-testid="save">保存</button>',
  },
  global: { stubs: { Teleport: true } },
})

describe('BaseModal', () => {
  it('renders an accessible dialog with title and slots when open', () => {
    const wrapper = mountModal({ open: true, title: '编辑应用' })
    const dialog = wrapper.get('[role="dialog"]')

    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(dialog.attributes('aria-labelledby')).toMatch(/^base-modal-title-/)
    expect(wrapper.get('h2').text()).toBe('编辑应用')
    expect(wrapper.get('[data-testid="body"]').text()).toBe('内容')
    expect(wrapper.get('[data-testid="save"]').text()).toBe('保存')
  })

  it('emits close for the close button, overlay, and Escape', async () => {
    const wrapper = mountModal({ open: true, title: '详情' })

    await wrapper.get('.base-modal-close').trigger('click')
    await wrapper.get('.base-modal-overlay').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(3)
  })

  it('does not close from configured disabled interactions', async () => {
    const wrapper = mountModal({ open: true, title: '详情', closeOnOverlay: false, closeOnEscape: false })

    await wrapper.get('.base-modal-overlay').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
