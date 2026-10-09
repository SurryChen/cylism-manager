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
    ['/resources', '/cluster?tab=list'],
    ['/workloads', '/cluster?tab=list'],
    ['/services', '/cluster?tab=list'],
    ['/configs', '/cluster?tab=list'],
    ['/network', '/cluster?tab=list'],
    ['/storage', '/cluster?tab=list'],
    ['/monitoring', '/cluster?tab=list'],
    ['/routes', '/cluster?tab=list'],
    ['/certs', '/cluster?tab=list'],
    ['/certs/default/example', '/cluster?tab=list'],
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

  it('opens Kubernetes resources for the selected cluster', async () => {
    await router.push('/cluster/42/resources?tab=services')
    expect(router.currentRoute.value.params.clusterID).toBe('42')
    expect(router.currentRoute.value.query.tab).toBe('services')
  })

  it.each(['/cluster/42/network', '/cluster/42/storage', '/cluster/42/monitoring'])('opens %s for the selected cluster', async path => {
    await router.push(path)
    expect(router.currentRoute.value.params.clusterID).toBe('42')
    expect(router.currentRoute.value.path).toBe(path)
  })

  it.each(['/cloud-services/domains', '/cloud-services/object-storage'])('loads %s from cloud services', async path => {
    const resourceRoute = router.getRoutes().find(route => route.path === path)
    expect(resourceRoute).toBeDefined()
    await router.push(path)
    expect(router.currentRoute.value.path).toBe(path)
  })

})
