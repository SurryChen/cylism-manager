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
    ['/cluster/registry-mirrors', '/cluster'],
    ['/cluster/chart-repositories', '/cluster'],
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

  it('opens a cluster detail route separately from the cluster tabs', async () => {
    await router.push('/cluster/42')
    expect(router.currentRoute.value.params.clusterID).toBe('42')
    expect(router.currentRoute.value.path).toBe('/cluster/42')
  })

  it.each(['/cloud-services/domains', '/cloud-services/object-storage'])('loads %s from cloud services', async path => {
    const resourceRoute = router.getRoutes().find(route => route.path === path)
    expect(resourceRoute).toBeDefined()
    await router.push(path)
    expect(router.currentRoute.value.path).toBe(path)
  })

})
