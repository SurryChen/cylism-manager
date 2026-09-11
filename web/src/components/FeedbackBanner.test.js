import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedbackBanner from './FeedbackBanner.vue'

describe('FeedbackBanner', () => {
  it.each(['error', 'warning', 'success', 'info'])('renders the %s tone and message', tone => {
    const wrapper = mount(FeedbackBanner, {
      props: { tone, message: `${tone} message` },
    })

    expect(wrapper.classes()).toContain(`feedback-banner--${tone}`)
    expect(wrapper.text()).toContain(`${tone} message`)
  })

  it('supports custom content and dismiss events', async () => {
    const wrapper = mount(FeedbackBanner, {
      props: { tone: 'warning', dismissible: true },
      slots: { default: '<strong>自定义提示</strong>' },
    })

    expect(wrapper.get('[role="alert"]').text()).toContain('自定义提示')
    await wrapper.get('.feedback-banner-dismiss').trigger('click')
    expect(wrapper.emitted('dismiss')).toHaveLength(1)
  })
})
