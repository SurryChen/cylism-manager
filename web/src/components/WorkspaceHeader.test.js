import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import WorkspaceHeader from './WorkspaceHeader.vue'

describe('WorkspaceHeader', () => {
  it('renders a content title, description, and page-level actions', () => {
    const wrapper = mount(WorkspaceHeader, {
      props: { title: '镜像源规则', description: '管理节点的期望镜像规则。' },
      slots: { actions: '<button data-testid="create-mirror">新建镜像源</button>' },
    })

    expect(wrapper.get('h2').text()).toBe('镜像源规则')
    expect(wrapper.get('.workspace-header-description').text()).toBe('管理节点的期望镜像规则。')
    expect(wrapper.get('[data-testid="create-mirror"]').text()).toBe('新建镜像源')
  })

  it('omits optional description and actions', () => {
    const wrapper = mount(WorkspaceHeader, { props: { title: '审计记录' } })

    expect(wrapper.find('.workspace-header-description').exists()).toBe(false)
    expect(wrapper.find('.workspace-header-actions').exists()).toBe(false)
  })
})
