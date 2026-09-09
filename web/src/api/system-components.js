import { api } from './index.js'

export function getSystemComponents(options) { return api.get('/system-components', options) }
export function getClusterNodes(options) { return api.get('/nodes', options) }
