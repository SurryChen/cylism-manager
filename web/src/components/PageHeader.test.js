import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import PageHeader from './PageHeader.vue'

const themeCss = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')
const componentsCss = readFileSync(resolve(process.cwd(), 'src/styles/components.css'), 'utf8')

describe('PageHeader', () => {
  it('renders one page heading, description, and actions', () => {
    const wrapper = mount(PageHeader, {
      props: { title: '应用管理', description: '管理当前环境中的应用' },
      slots: { actions: '<button data-testid="create">创建应用</button>' },
    })

    expect(wrapper.get('h1').text()).toBe('应用管理')
    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.get('.page-header-description').text()).toBe('管理当前环境中的应用')
    expect(wrapper.get('[data-testid="create"]').text()).toBe('创建应用')
    expect(wrapper.get('.page-header-actions').element.parentElement).toBe(wrapper.element)
  })

  it('does not render optional content when it is not provided', () => {
    const wrapper = mount(PageHeader, { props: { title: '概览' } })

    expect(wrapper.find('.page-header-description').exists()).toBe(false)
    expect(wrapper.find('.page-header-actions').exists()).toBe(false)
    expect(wrapper.find('.page-header-back').exists()).toBe(false)
  })

  it('emits back with the configured destination', async () => {
    const wrapper = mount(PageHeader, {
      props: { title: '详情', backTo: '/applications', backLabel: '返回应用' },
    })

    await wrapper.get('.page-header-back').trigger('click')

    expect(wrapper.emitted('back')).toEqual([['/applications']])
    expect(wrapper.get('.page-header-back').text()).toBe('返回应用')
  })

  it('uses the shared standard-header layout tokens', () => {
    expect(themeCss).toContain('--page-header-height: 64px')
    expect(themeCss).toContain('--page-header-title-size: 25px')
    expect(themeCss).toContain('--page-header-content-offset: var(--space-16)')
    expect(componentsCss).toContain('min-height: var(--page-header-height)')
    expect(componentsCss).toContain('font-size: var(--page-header-title-size)')
    expect(componentsCss).toContain('margin-bottom: 0')
  })
})
