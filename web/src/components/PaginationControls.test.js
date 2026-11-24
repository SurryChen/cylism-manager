import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import PaginationControls from './PaginationControls.vue'

describe('PaginationControls', () => {
  it('shows the result range and emits page and size choices', async () => {
    const wrapper = mount(PaginationControls, { props: { page: 2, pageSize: 10, total: 35 } })
    expect(wrapper.text()).toContain('共 35 条，第 11–20 条')
    expect(wrapper.text()).toContain('2 / 4 页')
    await wrapper.get('[aria-label="末页"]').trigger('click')
    expect(wrapper.emitted('page-change')).toEqual([[4]])
    await wrapper.get('select[aria-label="每页条数"]').setValue('50')
    expect(wrapper.emitted('page-size-change')).toEqual([[50]])
    wrapper.unmount()
  })

  it('keeps size selection available for empty lists and disables invalid navigation', async () => {
    const wrapper = mount(PaginationControls)
    expect(wrapper.text()).toContain('共 0 条')
    expect(wrapper.text()).not.toContain('第 1–0 条')
    expect(wrapper.findAll('.icon-button').every(button => button.element.disabled)).toBe(true)
    expect(wrapper.get('.select-menu-trigger').element.disabled).toBe(false)
    await wrapper.get('select').setValue('')
    expect(wrapper.emitted('page-size-change')).toBeUndefined()
    wrapper.unmount()
  })

  it('disables all controls during loading and reports the final partial page', async () => {
    const wrapper = mount(PaginationControls, { props: { page: 4, pageSize: 10, total: 35, loading: true } })
    expect(wrapper.text()).toContain('第 31–35 条')
    expect(wrapper.findAll('button').every(button => button.element.disabled)).toBe(true)
    expect(wrapper.get('nav').attributes('aria-busy')).toBe('true')
    wrapper.unmount()
  })

  it('jumps to a specified page through the form and keeps the current page until confirmed', async () => {
    const wrapper = mount(PaginationControls, { props: { page: 2, total: 2630 } })
    const input = wrapper.get('[aria-label="跳转页码"]')
    await input.setValue('125')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('page-change')).toEqual([[125]])
    expect(wrapper.get('.pagination-controls-status').text()).toBe('2 / 263 页')
    await wrapper.setProps({ loading: true })
    expect(input.element.disabled).toBe(true)
    await wrapper.setProps({ page: 125, loading: false })
    expect(input.element.value).toBe('125')
    expect(wrapper.get('button[type="submit"]').element.disabled).toBe(true)
    wrapper.unmount()
  })

  it('rejects empty, fractional, out-of-range and unchanged target pages', async () => {
    const wrapper = mount(PaginationControls, { props: { page: 2, total: 35 }, attachTo: document.body })
    const input = wrapper.get('[aria-label="跳转页码"]')
    for (const value of ['', '0', '-1', '1.5', '5', '2']) {
      await input.setValue(value)
      expect(wrapper.get('button[type="submit"]').element.disabled).toBe(true)
      await wrapper.get('form').trigger('submit')
    }
    expect(wrapper.emitted('page-change')).toBeUndefined()
    await input.setValue('4')
    expect(input.attributes('aria-invalid')).toBe('false')
    await wrapper.get('button[type="submit"]').trigger('click')
    expect(wrapper.emitted('page-change')).toEqual([[4]])
    await wrapper.setProps({ total: 10, page: 1 })
    expect(input.element.value).toBe('1')
    expect(input.element.disabled).toBe(true)
    wrapper.unmount()
  })
})
