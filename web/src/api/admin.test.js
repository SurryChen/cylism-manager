import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getAdminTableRows, getAdminTables } from './admin.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('admin api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes table pagination parameters', () => {
    const options = { signal: new AbortController().signal }
    getAdminTableRows('audit logs', { page: 2, size: 20, sort: 'created_at', order: 'desc' }, options)
    expect(api.get).toHaveBeenCalledWith('/admin/tables/audit%20logs?page=2&size=20&sort=created_at&order=desc', options)
  })

  it('loads the table catalog', () => {
    getAdminTables()
    expect(api.get).toHaveBeenCalledWith('/admin/tables', undefined)
  })
})
