import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createImageRegistry, deleteImageRegistry, getImageRegistryResources, updateImageRegistry, verifyImageRegistry } from './image-registries.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('image registries api', () => {
  it('owns global registry resources and mutations', () => {
    const options = { signal: new AbortController().signal }
    const body = { name: 'harbor' }
    getImageRegistryResources(options)
    createImageRegistry(body, options)
    updateImageRegistry('registry/1', body, options)
    verifyImageRegistry('registry/1', options)
    deleteImageRegistry('registry/1', options)

    expect(api.get).toHaveBeenNthCalledWith(1, '/image-registries', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/projects', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/image-registries', body, options)
    expect(api.put).toHaveBeenCalledWith('/image-registries/registry%2F1', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/image-registries/registry%2F1/verify', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/image-registries/registry%2F1', undefined, options)
  })
})
