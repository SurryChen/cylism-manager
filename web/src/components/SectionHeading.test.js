import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SectionHeading from './SectionHeading.vue'

describe('SectionHeading', () => {
  it('renders a heading, optional description, and actions', () => {
    const wrapper = mount(SectionHeading, {
      props: { title: '最近发布', description: '当前环境的最新发布记录' },
      slots: { actions: '<button data-testid="manage">管理</button>' },
    })

    expect(wrapper.get('h2').text()).toBe('最近发布')
    expect(wrapper.get('.section-heading-description').text()).toBe('当前环境的最新发布记录')
    expect(wrapper.get('[data-testid="manage"]').text()).toBe('管理')
  })

  it('omits empty description and actions regions', () => {
    const wrapper = mount(SectionHeading, { props: { title: '资源' } })

    expect(wrapper.find('.section-heading-description').exists()).toBe(false)
    expect(wrapper.find('.section-heading-actions').exists()).toBe(false)
  })
})
