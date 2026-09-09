import { api } from './index.js'

export function getIngressControllerStatus(options) { return api.get('/k8s/ingress-controller', options) }
export function getRoutes(options) { return api.get('/routes', options) }
export function getIngresses(options) { return api.get('/k8s/ingresses', options) }
