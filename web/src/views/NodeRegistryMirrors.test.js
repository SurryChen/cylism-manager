import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import NodeRegistryMirrors from './NodeRegistryMirrors.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

async function settle() {
  await new Promise(resolve => setTimeout(resolve, 0))
}

describe('Node registry mirrors view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue([{
      id: 1,
      name: 'docker-hub-mirror',
      registry: 'docker.io',
      endpoints: '["https://docker.1panel.live"]',
      verification_image: 'docker.io/library/busybox:1.36',
      enabled: true,
      last_verify_status: 'succeeded',
    }])
    api.post.mockResolvedValue({})
  })

  it('configures a verification image and triggers mirror detection', async () => {
    const wrapper = mount(NodeRegistryMirrors)
    await settle()

    expect(wrapper.text()).toContain('docker.io/library/busybox:1.36')
    expect(wrapper.text()).toContain('可用')

    await wrapper.get('[data-testid="verify-node-registry-mirror-1"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/1/verify')

    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(wrapper.get('input[placeholder="docker.io/library/busybox:1.36"]').exists()).toBe(true)
  })
})
