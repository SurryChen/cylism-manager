import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import SurfaceCard from './SurfaceCard.vue'

const source = readFileSync(resolve(process.cwd(), 'src/components/SurfaceCard.vue'), 'utf8')

describe('SurfaceCard', () => {
  it('renders a semantic glass panel with supplied content', () => {
    const wrapper = mount(SurfaceCard, {
      slots: { default: '<p data-testid="content">制品库内容</p>' },
    })

    expect(wrapper.element.tagName).toBe('SECTION')
    expect(wrapper.classes()).toContain('surface-card')
    expect(wrapper.classes()).toContain('surface-card--padding-md')
    expect(wrapper.get('[data-testid="content"]').text()).toBe('制品库内容')
  })

  it('supports header and actions regions without owning their content', () => {
    const wrapper = mount(SurfaceCard, {
      slots: {
        header: '<h2>镜像目录</h2>',
        actions: '<button data-testid="refresh">刷新</button>',
        default: '<div>仓库列表</div>',
      },
    })

    expect(wrapper.get('.surface-card-header h2').text()).toBe('镜像目录')
    expect(wrapper.get('.surface-card-actions [data-testid="refresh"]').text()).toBe('刷新')
    expect(wrapper.find('.surface-card-body').exists()).toBe(false)
    expect(wrapper.element.children[1].textContent).toContain('仓库列表')
  })

  it('offers structural variants without assigning click behavior', () => {
    const wrapper = mount(SurfaceCard, {
      props: { as: 'article', padding: 'none', interactive: true },
    })

    expect(wrapper.element.tagName).toBe('ARTICLE')
    expect(wrapper.classes()).toContain('surface-card--padding-none')
    expect(wrapper.classes()).toContain('surface-card--interactive')
    expect(wrapper.attributes('role')).toBeUndefined()
    expect(wrapper.attributes('tabindex')).toBeUndefined()
  })

  it('preserves aside semantics for complementary content panels', () => {
    const wrapper = mount(SurfaceCard, { props: { as: 'aside' } })

    expect(wrapper.element.tagName).toBe('ASIDE')
    expect(source).toContain("['section', 'article', 'aside', 'div']")
  })

  it('uses the shared glass surface tokens and respects reduced motion', () => {
    expect(source).toContain('background: var(--surface-glass)')
    expect(source).toContain('border: 1px solid var(--border)')
    expect(source).toContain('box-shadow: var(--shadow-soft)')
    expect(source).toContain('border-radius: var(--radius-panel)')
    expect(source).toContain('@media (prefers-reduced-motion: reduce)')
  })
})
