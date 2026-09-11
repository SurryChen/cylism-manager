import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from './EmptyState.vue'

describe('EmptyState', () => {
  it.each(['loading', 'empty', 'actionable'])('renders the %s variant and message', variant => {
    const wrapper = mount(EmptyState, {
      props: { variant, message: `${variant} message` },
    })

    expect(wrapper.classes()).toContain(`empty-state--${variant}`)
    expect(wrapper.get('.empty-state-message').text()).toBe(`${variant} message`)
  })

  it('renders an optional icon and action slot', () => {
    const wrapper = mount(EmptyState, {
      props: { message: '暂无应用', icon: '◌', variant: 'actionable' },
      slots: { action: '<button data-testid="create">创建应用</button>' },
    })

    expect(wrapper.get('.empty-state-icon').text()).toBe('◌')
    expect(wrapper.get('[data-testid="create"]').text()).toBe('创建应用')
  })
})
