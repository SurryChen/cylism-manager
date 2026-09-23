import { nextTick } from 'vue'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OverflowTooltip from './OverflowTooltip.vue'

describe('OverflowTooltip', () => {
  it('shows the full value only when its trigger is truncated', async () => {
    const wrapper = mount(OverflowTooltip, { props: { text: 'registry.k8s.io/very-long-image-reference' }, attachTo: document.body })
    const trigger = wrapper.get('.overflow-tooltip-trigger').element
    Object.defineProperties(trigger, { clientWidth: { configurable: true, value: 100 }, scrollWidth: { configurable: true, value: 240 } })

    await wrapper.get('.overflow-tooltip-trigger').trigger('mouseenter', { clientX: 80, clientY: 120 })
    await nextTick()
    expect(document.body.querySelector('.overflow-tooltip-content')?.textContent).toBe('registry.k8s.io/very-long-image-reference')

    await wrapper.get('.overflow-tooltip-trigger').trigger('mouseleave')
    expect(document.body.querySelector('.overflow-tooltip-content')).toBeNull()
    wrapper.unmount()
  })

  it('does not show a tooltip for a value that fits', async () => {
    const wrapper = mount(OverflowTooltip, { props: { text: 'docker.io' }, attachTo: document.body })
    const trigger = wrapper.get('.overflow-tooltip-trigger').element
    Object.defineProperties(trigger, { clientWidth: { configurable: true, value: 240 }, scrollWidth: { configurable: true, value: 100 } })

    await wrapper.get('.overflow-tooltip-trigger').trigger('mouseenter')
    expect(document.body.querySelector('.overflow-tooltip-content')).toBeNull()
    wrapper.unmount()
  })
})
