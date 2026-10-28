import { api } from './index.js'
import { createClusterKubernetesApi } from './kubernetes.js'

function queryString(params) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value) query.set(key, String(value))
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

function segment(value) {
  return encodeURIComponent(String(value))
}

function postRequest(path, body, options) {
  if (options !== undefined) return api.post(path, body, options)
  return body === undefined ? api.post(path) : api.post(path, body)
}

function putRequest(path, body, options) {
  return options === undefined ? api.put(path, body) : api.put(path, body, options)
}

function deleteRequest(path, options) {
  return options === undefined ? api.delete(path) : api.delete(path, undefined, options)
}

export function getApplications({ projectID, environmentID } = {}, options) {
  return api.get(`/applications${queryString({ project_id: projectID, environment_id: environmentID })}`, options)
}

export function getProjects(options) {
  return api.get('/projects', options)
}

export function getProjectEnvironments(projectID, options) {
  return api.get(`/projects/${segment(projectID)}/environments`, options)
}

export function getProjectEnvironmentNamespaceConflicts(options) {
  return api.get('/projects/environments/namespace-conflicts', options)
}

export function getProjectEnvironmentResources(projectID, options) {
  return Promise.all([
    getProjects(options),
    getApplications({}, options),
    getProjectEnvironmentNamespaceConflicts(options),
    getProjectEnvironments(projectID, options),
  ])
}

export function getImageRegistries({ projectID } = {}, options) {
  return api.get(`/image-registries${queryString({ project_id: projectID })}`, options)
}

export function getWorkspace(projectID, environmentID, options) {
  return api.get(`/workspace/overview${queryString({ project_id: projectID, environment_id: environmentID })}`, options)
}

export function getDomains({ unassigned = false } = {}, options) {
  return api.get(`/domains${queryString({ unassigned: unassigned || undefined })}`, options)
}

export function getApplication(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}`, options)
}

export function getApplicationDeployWebhook(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/deploy-webhook`, options)
}

export function generateApplicationDeployWebhookSecret(applicationID, options) {
  return postRequest(`/applications/${segment(applicationID)}/deploy-webhook/secret`, undefined, options)
}

export function disableApplicationDeployWebhook(applicationID, options) {
  return deleteRequest(`/applications/${segment(applicationID)}/deploy-webhook`, options)
}

export function getApplicationRuntime(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/runtime`, options)
}

export function getApplicationPods(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/pods`, options)
}

export function getApplicationPodLogs(applicationID, podName, { container, previous = false, tail = 200 }, options) {
  const query = new URLSearchParams({ container, tail: String(tail) })
  if (previous) query.set('previous', 'true')
  return api.get(`/applications/${segment(applicationID)}/pods/${segment(podName)}/logs?${query}`, options)
}

export function getDeploymentTemplates(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/deployment-templates`, options)
}

export function getApplicationCatalog(options) {
  return api.get('/application-catalog', options)
}

export function getApplicationCatalogItem(id, options) {
  return api.get(`/application-catalog/${segment(id)}`, options)
}

export function getApplicationCatalogInstallations(projectID, environmentID, options) {
  return api.get(`/application-catalog/installations${queryString({ project_id: projectID, environment_id: environmentID })}`, options)
}

export function createApplicationCatalogItem(payload, options) {
  return postRequest('/application-catalog', payload, options)
}

export function createApplicationCatalogFromEnvironment(payload, options) {
  return postRequest('/application-catalog/from-environment', payload, options)
}

export function updateApplicationCatalogItem(id, payload, options) {
  return putRequest(`/application-catalog/${segment(id)}`, payload, options)
}

export function deleteApplicationCatalogItem(id, options) {
  return deleteRequest(`/application-catalog/${segment(id)}`, options)
}

export function importApplicationCatalogItems(payload, options) {
  return postRequest('/application-catalog/import', payload, options)
}

export function getApplicationEndpoints(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/endpoints`, options)
}

export function getApplicationConfigResources(clusterID, namespace, options) {
  return createClusterKubernetesApi(clusterID).getConfigMapsForNamespace(namespace, options)
}

export function getApplicationSecretResources(clusterID, namespace, options) {
  return createClusterKubernetesApi(clusterID).getSecretsForNamespace(namespace, options)
}

export function createApplication({ projectID, environmentID, name }, options) {
  return postRequest('/applications', { project_id: projectID, environment_id: environmentID, name }, options)
}

export function deleteApplication(applicationID, options) {
  return deleteRequest(`/applications/${segment(applicationID)}`, options)
}

export function createProject(payload, options) {
  return postRequest('/projects', payload, options)
}

export function updateProject(projectID, payload, options) {
  return putRequest(`/projects/${segment(projectID)}`, payload, options)
}

export function deleteProject(projectID, options) {
  return deleteRequest(`/projects/${segment(projectID)}`, options)
}

export function createProjectEnvironment(projectID, body, options) {
  return postRequest(`/projects/${segment(projectID)}/environments`, body, options)
}

export function updateProjectEnvironment(projectID, environmentID, body, options) {
  return putRequest(`/projects/${segment(projectID)}/environments/${segment(environmentID)}`, body, options)
}

export function deleteProjectEnvironment(projectID, environmentID, options) {
  return deleteRequest(`/projects/${segment(projectID)}/environments/${segment(environmentID)}`, options)
}

export function syncProjectEnvironmentNamespace(projectID, environmentID, options) {
  return postRequest(`/projects/${segment(projectID)}/environments/${segment(environmentID)}/sync-namespace`, undefined, options)
}

export function createRelease(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/releases`, payload, options)
}

export function preflightRelease(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/releases/preflight`, payload, options)
}

export function getApplicationRelease(applicationID, releaseID, options) {
  return api.get(`/applications/${segment(applicationID)}/releases/${segment(releaseID)}`, options)
}

export function retryRelease(applicationID, releaseID, options) {
  return postRequest(`/applications/${segment(applicationID)}/releases/${segment(releaseID)}/retry`, undefined, options)
}

export function rollbackRelease(applicationID, releaseID, options) {
  return postRequest(`/applications/${segment(applicationID)}/releases/${segment(releaseID)}/rollback`, undefined, options)
}

export function saveApplicationCapabilities(applicationID, capabilities, options) {
  return putRequest(`/applications/${segment(applicationID)}/capabilities`, { capabilities }, options)
}

export function createIntegrationHandoff(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/integration-handoffs`, payload, options)
}

export function createDeploymentTemplate(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/deployment-templates`, payload, options)
}

export function updateDeploymentTemplate(applicationID, templateID, payload, options) {
  return putRequest(`/applications/${segment(applicationID)}/deployment-templates/${segment(templateID)}`, payload, options)
}

export function inspectDeploymentTemplateFile(applicationID, templateID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/deployment-templates/${segment(templateID)}/files/inspect`, payload, options)
}

export function deleteDeploymentTemplate(applicationID, templateID, options) {
  return deleteRequest(`/applications/${segment(applicationID)}/deployment-templates/${segment(templateID)}`, options)
}

export function setDefaultDeploymentTemplate(applicationID, templateID, options) {
  return postRequest(`/applications/${segment(applicationID)}/deployment-templates/${segment(templateID)}/default`, undefined, options)
}

export function createEndpoint(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/endpoints`, payload, options)
}

export function updateEndpoint(applicationID, endpointID, payload, options) {
  return putRequest(`/applications/${segment(applicationID)}/endpoints/${segment(endpointID)}`, payload, options)
}

export function deleteEndpoint(applicationID, endpointID, options) {
  return deleteRequest(`/applications/${segment(applicationID)}/endpoints/${segment(endpointID)}`, options)
}

export function restartApplication(applicationID, options) {
  return postRequest(`/applications/${segment(applicationID)}/restarts`, undefined, options)
}

export function updateWorkloadKind(applicationID, workloadKind, options) {
  return putRequest(`/applications/${segment(applicationID)}/workload-kind`, { workload_kind: workloadKind }, options)
}
