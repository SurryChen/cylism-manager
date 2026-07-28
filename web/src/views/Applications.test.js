import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Applications from './Applications.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn().mockResolvedValue([]), post: vi.fn().mockResolvedValue({ id: 1 }) }
}))

describe('Applications view', () => {
  it('keeps the application entry free of a list frame when there is no data', async () => {
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('应用')
    expect(wrapper.text()).not.toContain('暂无应用')
    expect(wrapper.text()).toContain('创建应用')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows project and environment management when opened from its secondary navigation item', async () => {
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('项目与环境')
    expect(wrapper.text()).toContain('新建项目')
    expect(wrapper.text()).not.toContain('暂无项目')
    expect(wrapper.text()).not.toContain('的环境')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows a cross-application release history section', async () => {
    const wrapper = mount(Applications, { props: { section: 'releases' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('发布记录')
    expect(wrapper.text()).not.toContain('暂无发布记录')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows each project with its environments and namespaces', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') {
        return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      }
      return Promise.resolve([{ id: 3, project_id: 1, environment_id: 2, name: 'order-api' }])
    })
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('production')
    expect(wrapper.text()).toContain('commerce-prod')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('删除')
  })
})
