import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ErrorNoticeModal from './ErrorNoticeModal.vue'

describe('ErrorNoticeModal', () => {
  it('presents an error message and can be dismissed', async () => {
    const wrapper = mount(ErrorNoticeModal, { props: { open: true, title: '加载失败', message: '无法读取集群状态' } })

    expect(wrapper.get('[role="dialog"]').text()).toContain('加载失败')
    expect(wrapper.get('[role="alert"]').text()).toContain('无法读取集群状态')
    await wrapper.get('[data-testid="dismiss-error-notice"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
