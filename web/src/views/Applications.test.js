import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Applications from './Applications.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn().mockResolvedValue([]), post: vi.fn().mockResolvedValue({ id: 1 }) }
}))

describe('Applications view', () => {
  it('renders the application release entry and empty state', async () => {
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('应用')
    expect(wrapper.text()).toContain('暂无应用')
    expect(wrapper.text()).toContain('创建应用')
  })
})
