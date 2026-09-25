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

  it('moves the Registry Proxy tab URL into cloud services while preserving its tab query', async () => {
    await router.push('/delivery/registry?tab=registry-proxy')
    expect(router.currentRoute.value.fullPath).toBe('/cloud-services/registry?tab=registry-proxy')
  })

  it('loads the registry workspace from its cloud services route on demand', async () => {
    const registryRoute = router.getRoutes().find(route => route.path === '/cloud-services/registry')
    expect(registryRoute).toBeDefined()

    await router.push('/cloud-services/registry')

    expect(router.currentRoute.value.path).toBe('/cloud-services/registry')
  })

})
