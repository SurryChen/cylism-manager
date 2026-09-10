import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChartRepositories from './ChartRepositories.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  api.get.mockResolvedValue([])
  api.post.mockResolvedValue({ id: 1 })
  api.put.mockResolvedValue({ id: 1 })
  api.delete.mockResolvedValue({})
})

describe('ChartRepositories view', () => {
  it('creates a chart repository through the named contract', async () => {
    const wrapper = mount(ChartRepositories)
    await flushPromises()
    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="jetstack"]').setValue('jetstack')
    await wrapper.get('input[placeholder="https://charts.jetstack.io"]').setValue('https://charts.jetstack.io')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.post).toHaveBeenCalledWith('/chart-repositories', {
      name: 'jetstack', endpoint: 'https://charts.jetstack.io', chart_name: 'cert-manager', chart_version: 'v1.16.3', enabled: true,
    })
  })

  it('keeps the edit form open when saving fails', async () => {
    api.post.mockRejectedValueOnce(new Error('Chart 仓库不可达'))
    const wrapper = mount(ChartRepositories)
    await flushPromises()
    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="jetstack"]').setValue('jetstack')
    await wrapper.get('input[placeholder="https://charts.jetstack.io"]').setValue('https://charts.jetstack.io')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.get('input[placeholder="jetstack"]').element.value).toBe('jetstack')
    expect(wrapper.text()).toContain('Chart 仓库不可达')
  })
})
