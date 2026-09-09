import { api } from './index.js'

function queryString(params) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value) query.set(key, String(value))
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
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
  return api.get(`/applications/${applicationID}`, options)
}

export function getDeploymentTemplates(applicationID, options) {
  return api.get(`/applications/${applicationID}/deployment-templates`, options)
}
