import { describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { api } from '../api/index.js'
import { useListPage } from './useListPage.js'
vi.mock('../api/index.js', () => ({ api: { get: vi.fn() } }))

describe('management list pagination', () => {
  it('sends real query parameters, retains failed pages, clamps deletes and resets scope', async () => {
    let rows = Array.from({ length: 24 }, (_, id) => ({ id: id + 1 }))
    let fail = false
    api.get.mockImplementation(async (path, options) => {
      expect(options.signal).toBeInstanceOf(AbortSignal)
      if (fail) throw new Error('读取失败')
      const query = new URL(path, 'http://localhost').searchParams
      const offset = Number(query.get('offset')), limit = Number(query.get('limit'))
      return { items: rows.slice(offset, offset + limit), total: rows.length }
    })
    const scope = effectScope(), visible = ref([]), error = ref('')
    const page = scope.run(() => useListPage('/servers', visible, error))
    await page.refresh({ server_ids: '1,2,3' }); await nextTick()
    expect(visible.value).toHaveLength(10)
    expect(api.get).toHaveBeenLastCalledWith('/servers?server_ids=1%2C2%2C3&limit=10&offset=0', expect.anything())
    await page.changePage(3); await nextTick()
    expect(visible.value.map(row => row.id)).toEqual([21, 22, 23, 24])
    fail = true
    await page.changePage(2); await nextTick()
    expect(page.page.value).toBe(3)
    expect(visible.value[0].id).toBe(21)
    expect(error.value).toBe('读取失败')
    fail = false; rows = rows.slice(0, 20)
    await page.refresh(); await nextTick()
    expect(page.page.value).toBe(2)
    expect(visible.value[0].id).toBe(11)
    await page.changePageSize(20); await nextTick()
    expect(page.page.value).toBe(1)
    expect(visible.value).toHaveLength(20)
    await page.refresh({ server_ids: '4,5' })
    expect(api.get).toHaveBeenLastCalledWith('/servers?server_ids=4%2C5&limit=20&offset=0', expect.anything())
    scope.stop()
  })

  it('cancels an old scope without overwriting a newer page', async () => {
    let finishOld, oldSignal
    api.get.mockImplementation((path, options) => {
      if (path.includes('cluster_id=1')) {
        oldSignal = options.signal
        return new Promise(resolve => { finishOld = resolve })
      }
      return Promise.resolve({ items: [{ id: 2 }], total: 1 })
    })
    const scope = effectScope(), rows = ref([]), error = ref('')
    const page = scope.run(() => useListPage('/servers', rows, error))
    const old = page.refresh({ cluster_id: 1 })
    await page.refresh({ cluster_id: 2 })
    finishOld({ items: [{ id: 1 }], total: 1 }); await old; await nextTick()
    expect(oldSignal.aborted).toBe(true)
    expect(rows.value).toEqual([{ id: 2 }])
    scope.stop()
  })
})
