import { api } from './index.js'
export { getClusterNodes } from './cluster.js'

export function getSystemComponents(options) { return api.get('/system-components', options) }
