import { api } from './index.js'

function segment(value) { return encodeURIComponent(String(value)) }
function post(path, body, options) { return options ? api.post(path, body, options) : api.post(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }
function clusterPath(clusterID, path) { return clusterID ? `/clusters/${encodeURIComponent(clusterID)}${path}` : path }

export function getIngressControllerStatus(options, clusterID) { return api.get(clusterPath(clusterID, '/k8s/ingress-controller'), options) }
export function getRoutes(options) { return api.get('/routes', options) }
export function getIngresses(options, clusterID) { return api.get(clusterPath(clusterID, '/k8s/ingresses'), options) }
export function createIngress(body, options, clusterID) { return post(clusterPath(clusterID, '/k8s/ingresses'), body, options) }
export function deleteRoute(namespace, name, options) { return remove(`/routes/${segment(namespace)}/${segment(name)}`, options) }
export function deleteIngress(namespace, name, options, clusterID) { return remove(clusterPath(clusterID, `/k8s/ingresses/${segment(namespace)}/${segment(name)}`), options) }
