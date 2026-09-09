import { api } from './index.js'
import { getConfigMapsForNamespace, getSecretsForNamespace } from './kubernetes.js'

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

export function getDeploymentTemplates(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/deployment-templates`, options)
}

export function getApplicationEndpoints(applicationID, options) {
  return api.get(`/applications/${segment(applicationID)}/endpoints`, options)
}

export function getApplicationConfigResources(namespace, options) {
  return getConfigMapsForNamespace(namespace, options)
}

export function getApplicationSecretResources(namespace, options) {
  return getSecretsForNamespace(namespace, options)
}

export function createApplication({ projectID, environmentID, name }, options) {
  return postRequest('/applications', { project_id: projectID, environment_id: environmentID, name }, options)
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

export function createRelease(applicationID, payload, options) {
  return postRequest(`/applications/${segment(applicationID)}/releases`, payload, options)
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
