import { beforeEach, describe, expect, it } from 'vitest'
import router from './index.js'

describe('infrastructure route migration', () => {
  beforeEach(() => localStorage.setItem('access_token', 'test-token'))

  it('loads page components on demand', () => {
    const componentRoutes = router.getRoutes().filter(route => route.components?.default)

    expect(componentRoutes).not.toHaveLength(0)
    for (const route of componentRoutes) {
      expect(route.components.default).toEqual(expect.any(Function))
    }
  })

  it.each([
    ['/cluster/registry-mirrors', '/cluster?tab=registry-mirrors'],
    ['/cluster/chart-repositories', '/cluster?tab=chart-repositories'],
    ['/workloads', '/resources?tab=workloads'],
    ['/services', '/resources?tab=services'],
    ['/configs', '/resources?tab=configs'],
    ['/routes', '/network?tab=routes'],
    ['/certs', '/network?tab=certificates'],
    ['/certs/default/example', '/network/certificates/default/example'],
  ])('redirects %s to %s', async (oldPath, expectedPath) => {
    await router.push(oldPath)
    expect(router.currentRoute.value.fullPath).toBe(expectedPath)
  })

})
