import { computed, unref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export function useRoutedTab({
  tabs,
  defaultTab,
  path,
  route: providedRoute,
  router: providedRouter,
}) {
  const route = providedRoute ?? useRoute()
  const router = providedRouter ?? useRouter()
  const tabItems = computed(() => unref(tabs) || [])

  const activeTab = computed(() => {
    const requestedTab = route.query?.tab
    return tabItems.value.some(item => item.id === requestedTab)
      ? requestedTab
      : defaultTab
  })

  function selectTab(tab) {
    if (!tabItems.value.some(item => item.id === tab)) return undefined

    return router.push({
      path: path || route.path,
      query: { ...route.query, tab },
    })
  }

  return { activeTab, selectTab }
}
