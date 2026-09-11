import { api } from './index.js'

function segment(value) { return encodeURIComponent(String(value)) }
function post(path, body, options) { return options ? api.post(path, body, options) : api.post(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getIngressControllerStatus(options) { return api.get('/k8s/ingress-controller', options) }
export function getRoutes(options) { return api.get('/routes', options) }
export function getIngresses(options) { return api.get('/k8s/ingresses', options) }
export function createIngress(body, options) { return post('/k8s/ingresses', body, options) }
export function deleteRoute(namespace, name, options) { return remove(`/routes/${segment(namespace)}/${segment(name)}`, options) }
export function deleteIngress(namespace, name, options) { return remove(`/k8s/ingresses/${segment(namespace)}/${segment(name)}`, options) }
