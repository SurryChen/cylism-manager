import { beforeEach, describe, expect, it } from 'vitest'
import router from './index.js'

describe('infrastructure route migration', () => {
  beforeEach(() => localStorage.setItem('access_token', 'test-token'))

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

  it('registers the model configuration page', async () => {
    await router.push('/settings/models')
    expect(router.currentRoute.value.fullPath).toBe('/settings/models')
  })
})
