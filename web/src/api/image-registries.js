import { api } from './index.js'

function segment(value) { return encodeURIComponent(String(value)) }
function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getImageRegistries(options) { return api.get('/image-registries', options) }
export function getImageRegistryResources(options) { return Promise.all([getImageRegistries(options), api.get('/projects', options)]) }
export function createImageRegistry(body, options) { return post('/image-registries', body, options) }
export function updateImageRegistry(id, body, options) { return put(`/image-registries/${segment(id)}`, body, options) }
export function verifyImageRegistry(id, options) { return post(`/image-registries/${segment(id)}/verify`, undefined, options) }
export function deleteImageRegistry(id, options) { return remove(`/image-registries/${segment(id)}`, options) }
