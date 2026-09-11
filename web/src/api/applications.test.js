import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  createApplication,
  createDeploymentTemplate,
  createEndpoint,
  createIntegrationHandoff,
  createProject,
  createRelease,
  deleteDeploymentTemplate,
  deleteEndpoint,
  deleteProject,
  getApplicationConfigResources,
  getApplicationEndpoints,
  getApplicationRelease,
  getApplications,
  getProjectEnvironmentResources,
  getWorkspace,
  restartApplication,
  retryRelease,
  rollbackRelease,
  saveApplicationCapabilities,
  setDefaultDeploymentTemplate,
  updateDeploymentTemplate,
  updateEndpoint,
  updateProject,
  updateProjectEnvironment,
  updateWorkloadKind,
  createProjectEnvironment,
  deleteProjectEnvironment,
  syncProjectEnvironmentNamespace,
} from './applications.js'

vi.mock('./index.js', () => ({ api: {
  delete: vi.fn(),
  get: vi.fn(),
  patch: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
} }))

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

  it('keeps application mutation contracts in the domain module', () => {
    const options = { signal: new AbortController().signal }
    createApplication({ projectID: 1, environmentID: 2, name: 'api' }, options)
    createProject({ name: 'commerce', description: 'Orders' }, options)
    updateProject(3, { name: 'commerce', description: 'Updated', default_image_registry_id: 4 }, options)
    deleteProject(3, options)
    createRelease(5, { template_id: 6, version: '1.2.3' }, options)
    restartApplication(5, options)
    saveApplicationCapabilities(5, ['metrics'], options)
    updateWorkloadKind(5, 'statefulset', options)

    expect(api.post).toHaveBeenNthCalledWith(1, '/applications', { project_id: 1, environment_id: 2, name: 'api' }, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/projects', { name: 'commerce', description: 'Orders' }, options)
    expect(api.put).toHaveBeenNthCalledWith(1, '/projects/3', { name: 'commerce', description: 'Updated', default_image_registry_id: 4 }, options)
    expect(api.delete).toHaveBeenCalledWith('/projects/3', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/applications/5/releases', { template_id: 6, version: '1.2.3' }, options)
    expect(api.post).toHaveBeenNthCalledWith(4, '/applications/5/restarts', undefined, options)
    expect(api.put).toHaveBeenNthCalledWith(2, '/applications/5/capabilities', { capabilities: ['metrics'] }, options)
    expect(api.put).toHaveBeenNthCalledWith(3, '/applications/5/workload-kind', { workload_kind: 'statefulset' }, options)
  })

  it('encodes detail actions and forwards options separately from payloads', () => {
    const options = { signal: new AbortController().signal }
    getApplicationRelease(7, 'rel/8', options)
    createIntegrationHandoff(7, { endpoint_id: 9, redirect_url: 'http://localhost:5178/?a=1' }, options)
    createDeploymentTemplate(7, { name: 'default' }, options)
    updateDeploymentTemplate(7, 10, { name: 'updated' }, options)
    deleteDeploymentTemplate(7, 10, options)
    setDefaultDeploymentTemplate(7, 10, options)
    createEndpoint(7, { domain_id: 1 }, options)
    updateEndpoint(7, 11, { path: '/v2' }, options)
    deleteEndpoint(7, 11, options)
    retryRelease(7, 'rel/8', options)
    rollbackRelease(7, 'rel/8', options)

    expect(api.get).toHaveBeenNthCalledWith(1, '/applications/7/releases/rel%2F8', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/applications/7/integration-handoffs', { endpoint_id: 9, redirect_url: 'http://localhost:5178/?a=1' }, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/applications/7/deployment-templates', { name: 'default' }, options)
    expect(api.put).toHaveBeenNthCalledWith(1, '/applications/7/deployment-templates/10', { name: 'updated' }, options)
    expect(api.delete).toHaveBeenNthCalledWith(1, '/applications/7/deployment-templates/10', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/applications/7/deployment-templates/10/default', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(4, '/applications/7/endpoints', { domain_id: 1 }, options)
    expect(api.put).toHaveBeenNthCalledWith(2, '/applications/7/endpoints/11', { path: '/v2' }, options)
    expect(api.delete).toHaveBeenNthCalledWith(2, '/applications/7/endpoints/11', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(5, '/applications/7/releases/rel%2F8/retry', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(6, '/applications/7/releases/rel%2F8/rollback', undefined, options)
  })

  it('owns project environment reads and mutations', () => {
    const options = { signal: new AbortController().signal }
    const body = { name: 'production', namespace: 'project-commerce' }
    getProjectEnvironmentResources('project/1', options)
    createProjectEnvironment('project/1', body, options)
    updateProjectEnvironment('project/1', 'environment/2', body, options)
    syncProjectEnvironmentNamespace('project/1', 'environment/2', options)
    deleteProjectEnvironment('project/1', 'environment/2', options)

    expect(api.get).toHaveBeenNthCalledWith(1, '/projects', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/applications', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/projects/environments/namespace-conflicts', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/projects/project%2F1/environments', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/projects/project%2F1/environments', body, options)
    expect(api.put).toHaveBeenCalledWith('/projects/project%2F1/environments/environment%2F2', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/projects/project%2F1/environments/environment%2F2/sync-namespace', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/projects/project%2F1/environments/environment%2F2', undefined, options)
  })
})
