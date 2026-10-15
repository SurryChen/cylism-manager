import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ObjectStorage from './ObjectStorage.vue'

const cloud = vi.hoisted(() => ({
  createCloudContainer: vi.fn(),
  deleteCloudContainer: vi.fn(),
  deleteCloudObject: vi.fn(),
  getCloudConnections: vi.fn(),
  getCloudContainers: vi.fn(),
  getCloudContainerCORS: vi.fn(),
  getCloudObjects: vi.fn(),
  updateCloudContainer: vi.fn(),
  updateCloudContainerCORS: vi.fn(),
  uploadCloudObject: vi.fn(),
  downloadCloudObjectURL: vi.fn(),
}))

vi.mock('../../api/cloud-resources.js', () => cloud)

beforeEach(() => {
  vi.clearAllMocks()
  cloud.getCloudConnections.mockResolvedValue([{ id: 8, name: '生产', provider: 'aliyun' }])
  cloud.getCloudContainers.mockResolvedValue([{ name: 'assets', region: 'cn-hangzhou', storage_class: 'Standard' }])
  cloud.getCloudObjects.mockResolvedValue({ objects: [{ key: 'logo.png', size: 10, last_modified: '2026-09-26T00:00:00Z' }] })
  cloud.getCloudContainerCORS.mockResolvedValue({ rules: [] })
  cloud.updateCloudContainerCORS.mockResolvedValue({})
  cloud.createCloudContainer.mockResolvedValue({})
  cloud.deleteCloudContainer.mockResolvedValue({})
  cloud.deleteCloudObject.mockResolvedValue({})
  cloud.downloadCloudObjectURL.mockReturnValue('/download')
})

describe('ObjectStorage', () => {
  it('keeps container creation available while the remote bucket list is pending', async () => {
    let resolveContainers
    cloud.getCloudContainers.mockReturnValue(new Promise(resolve => { resolveContainers = resolve }))
    const wrapper = mount(ObjectStorage)
    await flushPromises()

    expect(wrapper.text()).not.toContain('正在读取云连接')
    expect(wrapper.text()).toContain('正在读取存储容器')
    await wrapper.get('[data-testid=container-create]').trigger('click')
    expect(wrapper.get('[data-testid=container-region]').exists()).toBe(true)

    resolveContainers([])
    await flushPromises()
    wrapper.unmount()
  })

  it('uses the shared tabbed page header for its containers workspace', async () => {
    const wrapper = mount(ObjectStorage)
    await flushPromises()

    expect(wrapper.get('[data-header-variant="tabbed"] .page-title').text()).toBe('对象存储')
    expect(wrapper.findAll('.section-tab')).toHaveLength(1)
    expect(wrapper.get('[data-testid="object-storage-tab-containers"]').text()).toBe('存储容器')
    expect(wrapper.get('[data-testid="object-storage-tab-containers"]').classes()).toContain('is-active')
  })

  it('creates containers and deletes objects from the dedicated page', async () => {
    const wrapper = mount(ObjectStorage)
    await flushPromises()

    await wrapper.get('[data-testid=container-create]').trigger('click')
    expect(wrapper.get('[data-testid=container-region]').element.value).toBe('')
    await wrapper.get('[data-testid=container-name]').setValue('backups')
    await wrapper.get('[data-testid=container-region]').setValue('cn-shanghai')
    await wrapper.get('[data-testid=container-save]').trigger('click')
    await flushPromises()
    expect(cloud.createCloudContainer).toHaveBeenCalledWith(8, expect.objectContaining({ name: 'backups', region: 'cn-shanghai' }))

    await wrapper.get('[data-testid=open-container-assets]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="object-delete-logo.png"]').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(cloud.getCloudObjects).toHaveBeenCalledWith(8, 'assets', { prefix: '', region: 'cn-hangzhou' })
    expect(cloud.deleteCloudObject).toHaveBeenCalledWith(8, 'assets', { key: 'logo.png', confirm: true }, 'cn-hangzhou')
  })

  it('opens objects in a modal instead of expanding below the container table', async () => {
    const wrapper = mount(ObjectStorage)
    await flushPromises()

    await wrapper.get('[data-testid=open-container-assets]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.object-modal').exists()).toBe(true)
    expect(wrapper.find('.object-modal').text()).toContain('logo.png')
    expect(wrapper.find('.objects-section').exists()).toBe(false)

    await wrapper.get('.object-modal .base-modal-close').trigger('click')
    expect(wrapper.find('.object-modal').exists()).toBe(false)
  })

  it('loads existing CORS rules and saves all of them for the bucket region', async () => {
    cloud.getCloudContainerCORS.mockResolvedValue({ rules: [
      { allowed_origins: ['https://one.example'], allowed_methods: ['GET'], allowed_headers: ['*'], expose_headers: [], max_age_seconds: 0 },
      { allowed_origins: ['https://two.example'], allowed_methods: ['POST'], allowed_headers: ['Content-Type'], expose_headers: ['ETag'], max_age_seconds: 600 },
    ] })
    const wrapper = mount(ObjectStorage)
    await flushPromises()
    await wrapper.get('[data-testid=container-cors-assets]').trigger('click')
    await flushPromises()
    expect(cloud.getCloudContainerCORS).toHaveBeenCalledWith(8, 'assets', 'cn-hangzhou')
    expect(wrapper.findAll('.cors-rule')).toHaveLength(2)
    await wrapper.get('[data-testid=cors-save]').trigger('click')
    await flushPromises()
    expect(cloud.updateCloudContainerCORS).toHaveBeenCalledWith(8, 'assets', expect.objectContaining({ rules: [
      expect.objectContaining({ allowed_origins: ['https://one.example'] }),
      expect.objectContaining({ allowed_origins: ['https://two.example'], allowed_methods: ['POST'] }),
    ] }), 'cn-hangzhou')
  })

  it('removes bucket CORS when access is disabled', async () => {
    cloud.getCloudContainerCORS.mockResolvedValue({ rules: [{ allowed_origins: ['*'], allowed_methods: ['GET'] }] })
    const wrapper = mount(ObjectStorage)
    await flushPromises()
    await wrapper.get('[data-testid=container-cors-assets]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid=cors-enabled]').setValue(false)
    await wrapper.get('[data-testid=cors-save]').trigger('click')
    await flushPromises()
    expect(cloud.updateCloudContainerCORS).toHaveBeenCalledWith(8, 'assets', expect.objectContaining({ rules: [] }), 'cn-hangzhou')
  })
})
