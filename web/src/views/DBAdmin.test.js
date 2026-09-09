import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import DBAdmin from './DBAdmin.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}))

describe('DB admin view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockImplementation(path => {
      if (path === '/admin/tables') return Promise.resolve({ tables: ['users'] })
      return Promise.resolve({ columns: ['id', 'name'], rows: [{ id: 1, name: 'admin' }], total: 1 })
    })
  })

  it('loads the table catalog and selected table through managed API calls', async () => {
    const wrapper = mount(DBAdmin)
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()
    expect(wrapper.text()).toContain('用户')

    await wrapper.get('.tab-btn').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()
    expect(api.get).toHaveBeenCalledWith('/admin/tables/users?page=1&size=20&sort=id&order=desc', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('admin')
  })

  it('shows a local error when table data cannot be loaded', async () => {
    api.get.mockImplementation(path => path === '/admin/tables' ? Promise.resolve({ tables: ['users'] }) : Promise.reject(new Error('database unavailable')))
    const wrapper = mount(DBAdmin)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('.tab-btn').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()
    expect(wrapper.text()).toContain('database unavailable')
  })
})
