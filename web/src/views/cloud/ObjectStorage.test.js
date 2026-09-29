import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ObjectStorage from './ObjectStorage.vue'

const cloud = vi.hoisted(() => ({
  createCloudContainer: vi.fn(),
  deleteCloudContainer: vi.fn(),
  deleteCloudObject: vi.fn(),
  getCloudConnections: vi.fn(),
  getCloudContainers: vi.fn(),
  getCloudObjects: vi.fn(),
  updateCloudContainer: vi.fn(),
  uploadCloudObject: vi.fn(),
  downloadCloudObjectURL: vi.fn(),
}))

vi.mock('../../api/cloud-resources.js', () => cloud)

beforeEach(() => {
  vi.clearAllMocks()
  cloud.getCloudConnections.mockResolvedValue([{ id: 8, name: '生产', provider: 'aliyun' }])
  cloud.getCloudContainers.mockResolvedValue([{ name: 'assets', region: 'cn-hangzhou', storage_class: 'Standard' }])
  cloud.getCloudObjects.mockResolvedValue({ objects: [{ key: 'logo.png', size: 10, last_modified: '2026-09-26T00:00:00Z' }] })
  cloud.createCloudContainer.mockResolvedValue({})
  cloud.deleteCloudContainer.mockResolvedValue({})
  cloud.deleteCloudObject.mockResolvedValue({})
  cloud.downloadCloudObjectURL.mockReturnValue('/download')
})

describe('ObjectStorage', () => {
  it('creates containers and deletes objects from the dedicated page', async () => {
    const wrapper = mount(ObjectStorage)
    await flushPromises()

    await wrapper.get('[data-testid=container-create]').trigger('click')
    await wrapper.get('[data-testid=container-name]').setValue('backups')
    await wrapper.get('[data-testid=container-save]').trigger('click')
    await flushPromises()
    expect(cloud.createCloudContainer).toHaveBeenCalledWith(8, expect.objectContaining({ name: 'backups' }))

    await wrapper.get('[data-testid=open-container-assets]').trigger('click')
    await flushPromises()
    vi.stubGlobal('confirm', vi.fn(() => true))
    await wrapper.get('[data-testid="object-delete-logo.png"]').trigger('click')
    await flushPromises()
    expect(cloud.deleteCloudObject).toHaveBeenCalledWith(8, 'assets', { key: 'logo.png', confirm: true })
  })
})
