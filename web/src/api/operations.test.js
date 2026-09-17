import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getOperations } from './operations.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('operations api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes global history filters', () => {
    const options = { signal: new AbortController().signal }
    getOperations({ resourceType: 'application', status: 'failed', keyword: 'worker', limit: 20, offset: 40 }, options)
    expect(api.get).toHaveBeenCalledWith('/operations?limit=20&offset=40&resource_type=application&status=failed&keyword=worker', options)
  })
})
