import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PersistentVolumes from './PersistentVolumes.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

describe('PersistentVolumes view', () => {
  it('creates a ReadWriteOnce PVC in the selected environment', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'knowledge', environments: [{ id: 2, name: 'production', namespace: 'project-knowledge-prod' }] }])
      if (path.startsWith('/k8s/persistent-volume-claims')) return Promise.resolve([])
      if (path === '/k8s/storage-classes') return Promise.resolve([{ name: 'local-path', is_default: true, volume_binding_mode: 'WaitForFirstConsumer' }])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue({})
    const wrapper = mount(PersistentVolumes)
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('select').setValue('1')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.text()).toContain('当前环境还没有平台托管存储卷')

    await wrapper.get('.btn-primary').trigger('click')
    await wrapper.get('input[placeholder="karakeep-data"]').setValue('karakeep-data')
    await wrapper.get('form').trigger('submit')

    expect(api.post).toHaveBeenCalledWith('/k8s/persistent-volume-claims', {
      environment_id: 2, name: 'karakeep-data', storage: '5Gi', storage_class_name: '',
    })
  })
})
