import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TabbedWorkspaceCard from './TabbedWorkspaceCard.vue'

describe('TabbedWorkspaceCard', () => {
  it('places context and actions in a shared card toolbar', () => {
    const wrapper = mount(TabbedWorkspaceCard, {
      slots: {
        title: '<h2>规则</h2>',
        meta: '<span>3 条</span>',
        actions: '<button>新建</button>',
        default: '<div class="table-wrap">内容</div>',
      },
    })

    expect(wrapper.get('.tabbed-workspace-toolbar').text()).toContain('规则')
    expect(wrapper.get('.tabbed-workspace-toolbar').text()).toContain('3 条')
    expect(wrapper.get('.tabbed-workspace-actions').text()).toContain('新建')
    expect(wrapper.getComponent({ name: 'SurfaceCard' }).classes()).toContain('surface-card--padding-none')
  })
})
