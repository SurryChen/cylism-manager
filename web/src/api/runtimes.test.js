import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getRuntimeAgentState, getRuntimeCatalog, getRuntimes } from './runtimes.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('runtimes api', () => {
  it('loads runtime resources with a shared signal', () => {
    const options = { signal: new AbortController().signal }
    getRuntimeCatalog(options)
    getRuntimes(options)
    getRuntimeAgentState(4, options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/runtimes/catalog', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/runtimes', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/runtimes/4/agent-capability-grants', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/runtimes/4/agent-operations', options)
  })
})
