import { api } from './index.js'

export function getRuntimes(options) { return api.get('/runtimes', options) }
export function getRuntimeCatalog(options) { return api.get('/runtimes/catalog', options) }
export function getRuntimeNodes(options) { return api.get('/nodes', options) }
export function getRuntimeNamespaces(options) { return api.get('/k8s/namespace-names', options) }
export function getRuntimeAgentState(runtimeID, options) {
  return Promise.all([
    api.get(`/runtimes/${runtimeID}/agent-capability-grants`, options),
    api.get(`/runtimes/${runtimeID}/agent-operations`, options),
  ])
}
