import { reactive } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useRoutedTab } from './useRoutedTab.js'

const tabs = [
  { id: 'overview', label: '概览' },
  { id: 'details', label: '详情' },
]

function createRoute(query = {}) {
  return reactive({ path: '/resources', query: reactive({ ...query }) })
}

describe('useRoutedTab', () => {
  it('falls back to the default tab when the route tab is missing or invalid', () => {
    const route = createRoute()
    const router = { push: vi.fn() }
    const { activeTab } = useRoutedTab({
      route,
      router,
      tabs,
      defaultTab: 'overview',
      path: '/resources',
    })

    expect(activeTab.value).toBe('overview')
    route.query.tab = 'unknown'
    expect(activeTab.value).toBe('overview')
  })

  it('updates the tab while preserving unrelated query parameters', async () => {
    const route = createRoute({ namespace: 'project-demo', tab: 'overview' })
    const router = { push: vi.fn().mockResolvedValue(undefined) }
    const { selectTab } = useRoutedTab({
      route,
      router,
      tabs,
      defaultTab: 'overview',
      path: '/resources',
    })

    await selectTab('details')

    expect(router.push).toHaveBeenCalledWith({
      path: '/resources',
      query: { namespace: 'project-demo', tab: 'details' },
    })
  })

  it('ignores tabs that are not present in the configured tab list', async () => {
    const route = createRoute()
    const router = { push: vi.fn() }
    const { selectTab } = useRoutedTab({
      route,
      router,
      tabs,
      defaultTab: 'overview',
      path: '/resources',
    })

    await selectTab('missing')

    expect(router.push).not.toHaveBeenCalled()
  })
})
