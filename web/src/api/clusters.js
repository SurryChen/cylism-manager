import { api } from './index.js'

export function getClusters(options) {
  return api.get('/clusters', options)
}

export function getCluster(clusterID, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}`, options)
}

export function getClusterAgentStatus(clusterID, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/agent-status`, options)
}

export function createCluster(name, distribution) {
  return api.post('/clusters', { name, distribution })
}

export function updateCluster(clusterID, name) {
  return api.put(`/clusters/${encodeURIComponent(clusterID)}`, { name })
}

export function deleteCluster(clusterID) {
  return api.delete(`/clusters/${encodeURIComponent(clusterID)}`)
}

export function uninstallClusterNode(clusterID, serverID) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/nodes/${encodeURIComponent(serverID)}/uninstall`)
}

export function restartClusterNode(clusterID, serverID) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/nodes/${encodeURIComponent(serverID)}/restart`)
}

export function adoptClusterNode(clusterID, serverID, role) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/adoptions`, { server_id: Number(serverID), role })
}

export function getClusterDeployments(clusterID, { limit = 20, offset = 0 } = {}, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/deployments?limit=${limit}&offset=${offset}`, options)
}

export function getClusterDeploymentLogs(clusterID, deploymentID, after = 0, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/deployments/${encodeURIComponent(deploymentID)}/logs?after=${after}`, options)
}

export function startClusterDeployment(clusterID, serverID, role, mirror, networkMode = 'public', joinAddressType = '') {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/deployments`, {
    server_id: Number(serverID), role, mirror, network_mode: networkMode,
    ...(role === 'worker' ? { join_address_type: joinAddressType } : {}),
  })
}

export function startClusterAgentOperation(clusterID, serverID, operation) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/agent-operations`, { server_id: Number(serverID), operation })
}

export function checkClusterNetworkMigration(clusterID, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/network-migration/precheck`, options)
}

export function startClusterNetworkMigration(clusterID) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/network-migration`, {})
}

export function startClusterNetworkNodeMigration(clusterID, serverID) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/network-migration/nodes/${encodeURIComponent(serverID)}`, {})
}

export function getCrossClusterServices(clusterID, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/cross-services`, options)
}

export function getAllCrossClusterServices(options) {
  return api.get('/cross-cluster-services', options)
}

export function precheckCrossClusterService(clusterID, targetClusterID, options) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/cross-services/precheck?target_cluster_id=${encodeURIComponent(targetClusterID)}`, options)
}

export function createCrossClusterService(clusterID, payload) {
  return api.post(`/clusters/${encodeURIComponent(clusterID)}/cross-services`, payload)
}

export function checkCrossClusterService(clusterID, serviceID) {
  return api.get(`/clusters/${encodeURIComponent(clusterID)}/cross-services/${encodeURIComponent(serviceID)}/check`)
}

export function deleteCrossClusterService(clusterID, serviceID) {
  return api.delete(`/clusters/${encodeURIComponent(clusterID)}/cross-services/${encodeURIComponent(serviceID)}`)
}
