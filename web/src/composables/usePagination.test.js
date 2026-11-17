import { effectScope, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { usePagination } from './usePagination.js'

describe('usePagination', () => {
  it('defaults to ten rows, resets for size changes and clamps refreshed lists', () => {
    const scope = effectScope()
    const items = ref(Array.from({ length: 35 }, (_, index) => index + 1))
    const pagination = scope.run(() => usePagination(items))
    expect(pagination.pagedItems.value).toEqual(items.value.slice(0, 10))
    pagination.changePage(4)
    expect(pagination.pagedItems.value).toEqual([31, 32, 33, 34, 35])
    pagination.changePageSize(20)
    expect(pagination.page.value).toBe(1)
    expect(pagination.pagedItems.value).toHaveLength(20)
    pagination.changePage(2)
    items.value = [1, 2]
    expect(pagination.page.value).toBe(1)
    expect(pagination.pagedItems.value).toEqual([1, 2])
    pagination.changePage(0)
    pagination.changePage(99)
    pagination.changePageSize(0)
    expect(pagination.page.value).toBe(1)
    expect(pagination.pageSize.value).toBe(20)
    items.value = []
    expect(pagination.totalPages.value).toBe(1)
    expect(pagination.pagedItems.value).toEqual([])
    scope.stop()
  })
})
