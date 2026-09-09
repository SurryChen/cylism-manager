import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getApplicationConfigResources, getApplicationEndpoints, getApplications, getWorkspace } from './applications.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('applications API', () => {
  afterEach(() => vi.clearAllMocks())

  it('passes workspace identifiers and an abort signal to the existing endpoints', () => {
    const signal = new AbortController().signal

    getWorkspace(12, 34, { signal })
    getApplications({ projectID: 12, environmentID: 34 }, { signal })

    expect(api.get).toHaveBeenNthCalledWith(1, '/workspace/overview?project_id=12&environment_id=34', { signal })
    expect(api.get).toHaveBeenNthCalledWith(2, '/applications?project_id=12&environment_id=34', { signal })
  })

  it('builds application detail read endpoints', () => {
    const options = { signal: new AbortController().signal }
    getApplicationEndpoints(7, options)
    getApplicationConfigResources('team/ns', options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/applications/7/endpoints', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/k8s/configmaps?namespace=team%2Fns&usage=false', options)
  })
})
