import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmDialog from './ConfirmDialog.vue'

const mountDialog = props => mount(ConfirmDialog, {
  props,
  global: { stubs: { Teleport: true } },
})

describe('ConfirmDialog', () => {
  it('preserves the confirmation copy and emits confirm/cancel', async () => {
    const wrapper = mountDialog({
      open: true,
      title: '删除项目',
      message: '该操作不可撤销。',
      confirmText: '删除',
      cancelText: '保留',
    })

    expect(wrapper.get('[role="dialog"]').text()).toContain('该操作不可撤销。')
    await wrapper.get('[data-testid="confirm-dialog-cancel"]').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')

    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('confirm')).toHaveLength(1)
  })

  it('prevents repeated confirmation while busy', async () => {
    const wrapper = mountDialog({ open: true, title: '删除项目', message: '确认？', busy: true })

    expect(wrapper.get('[data-testid="confirm-dialog-confirm"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')

    expect(wrapper.emitted('confirm')).toBeUndefined()
  })
})
