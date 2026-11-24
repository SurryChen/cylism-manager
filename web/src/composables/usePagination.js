import { computed, ref, watch } from 'vue'

export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

// Paginate complete in-memory lists; clamp the page when refreshed data shrinks.
export function usePagination(items) {
  const page = ref(1)
  const pageSize = ref(10)
  const total = computed(() => items.value.length)
  const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
  const pagedItems = computed(() => items.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
  watch(totalPages, lastPage => { page.value = Math.min(page.value, lastPage) }, { flush: 'sync' })
  function changePage(value) {
    if (Number.isInteger(value) && value >= 1 && value <= totalPages.value) page.value = value
  }
  function changePageSize(value) {
    if (!PAGE_SIZE_OPTIONS.includes(value)) return
    page.value = 1
    pageSize.value = value
  }
  return { page, pageSize, total, totalPages, pagedItems, changePage, changePageSize }
}
