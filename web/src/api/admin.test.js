import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createAdminTableRow, deleteAdminTableRow, getAdminTableRows, getAdminTables, updateAdminTableRow } from './admin.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

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

  it('keeps table mutations in the admin API module', () => {
    const options = { signal: new AbortController().signal }
    createAdminTableRow('audit logs', { message: 'created' }, options)
    updateAdminTableRow('audit logs', 7, { message: 'updated' }, options)
    deleteAdminTableRow('audit logs', 7, options)
    expect(api.post).toHaveBeenCalledWith('/admin/tables/audit%20logs', { message: 'created' }, options)
    expect(api.put).toHaveBeenCalledWith('/admin/tables/audit%20logs/7', { message: 'updated' }, options)
    expect(api.delete).toHaveBeenCalledWith('/admin/tables/audit%20logs/7', undefined, options)
  })
})
