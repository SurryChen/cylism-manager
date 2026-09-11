import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createIngress, deleteIngress, deleteRoute } from './sites.js'

vi.mock('./index.js', () => ({ api: { post: vi.fn(), delete: vi.fn() } }))

describe('sites api', () => {
  it('owns route and ingress mutations with encoded path segments', () => {
    const options = { signal: new AbortController().signal }
    const body = { namespace: 'team/a', name: 'web' }
    createIngress(body, options)
    deleteRoute('team/a', 'route/name', options)
    deleteIngress('team/a', 'ingress/name', options)

    expect(api.post).toHaveBeenCalledWith('/k8s/ingresses', body, options)
    expect(api.delete).toHaveBeenNthCalledWith(1, '/routes/team%2Fa/route%2Fname', undefined, options)
    expect(api.delete).toHaveBeenNthCalledWith(2, '/k8s/ingresses/team%2Fa/ingress%2Fname', undefined, options)
  })
})
