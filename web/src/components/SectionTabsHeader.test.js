import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import SectionTabsHeader from './SectionTabsHeader.vue'

const themeCss = readFileSync(resolve(process.cwd(), 'src/styles/theme.css'), 'utf8')
const sectionTabsSource = readFileSync(resolve(process.cwd(), 'src/components/SectionTabsHeader.vue'), 'utf8')

const tabs = [
  { id: 'security', label: '安全与访问' },
  { id: 'entry', label: '平台入口' },
]

describe('SectionTabsHeader', () => {
  it('keeps one heading and its local tabs in the same primary header', () => {
    const wrapper = mount(SectionTabsHeader, {
      props: { title: '系统设置', tabs, activeTab: 'security', testIdPrefix: 'settings' },
      slots: { actions: '<span data-testid="header-action">已保存</span>' },
    })

    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.get('h1').text()).toBe('系统设置')
    expect(wrapper.get('nav').element.parentElement).toBe(wrapper.element)
    expect(wrapper.get('[data-testid="header-action"]').element.parentElement?.parentElement).toBe(wrapper.element)
    expect(wrapper.get('[data-testid="settings-security"]').attributes('aria-current')).toBe('page')
  })

  it('emits the selected local tab without creating route navigation', async () => {
    const wrapper = mount(SectionTabsHeader, {
      props: { title: '系统设置', tabs, activeTab: 'security', testIdPrefix: 'settings' },
    })

    await wrapper.get('[data-testid="settings-entry"]').trigger('click')

    expect(wrapper.emitted('select')).toEqual([['entry']])
  })

  it('uses shared tabbed-header layout tokens', () => {
    expect(themeCss).toContain('--tabbed-page-header-height: var(--page-header-height)')
    expect(themeCss).toContain('--page-header-height: 64px')
    expect(themeCss).toContain('--tabbed-page-header-title-size: var(--page-header-title-size)')
    expect(themeCss).toContain('--tabbed-page-header-content-offset: var(--page-header-content-offset)')
    expect(sectionTabsSource).toContain('height: var(--tabbed-page-header-height)')
    expect(sectionTabsSource).toContain('margin: 0 0 23px; font-size: var(--tabbed-page-header-title-size)')
    expect(sectionTabsSource).toContain('bottom: 11px; left: 0; height: 1px')
    expect(sectionTabsSource).toContain('font-size: var(--tabbed-page-header-title-size)')
    expect(sectionTabsSource).toContain('transition: opacity var(--page-header-motion-duration) ease, transform var(--page-header-motion-duration) ease')
  })
})
