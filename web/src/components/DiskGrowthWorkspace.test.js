import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DiskGrowthWorkspace from './DiskGrowthWorkspace.vue'

const apiMocks = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('../api/index.js', () => ({ api: apiMocks }))

beforeEach(() => {
  apiMocks.get.mockReset()
  apiMocks.get.mockResolvedValue({
    mounts: [{ node: 'node-a', mount_point: '/var/lib', growth_bytes: 1048576 }],
    pvcs: [{ node: 'node-a', namespace: 'project-demo', pvc: 'data', growth_bytes: 2097152, consumers: ['app-1'] }],
    containers: [{ node: 'node-a', namespace: 'project-demo', pod: 'app-1', container: 'api', growth_bytes: 3145728 }],
  })
})

describe('DiskGrowthWorkspace', () => {
  it('loads diagnostics on mount and reloads with node and range filters', async () => {
    const wrapper = mount(DiskGrowthWorkspace, { props: { nodes: [{ name: 'node-a', display_name: '主节点', ready: true }] } })
    await flushPromises()

    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=6h')
    expect(wrapper.text()).toContain('/var/lib')
    expect(wrapper.text()).toContain('使用者: app-1')
    expect(wrapper.text()).toContain('3.0 MiB')

    const selects = wrapper.findAll('select')
    await selects[0].setValue('24h')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=24h')
    await selects[1].setValue('node-a')
    await flushPromises()
    expect(apiMocks.get).toHaveBeenCalledWith('/monitoring/disk-growth?range=24h&node=node-a')
  })
})
