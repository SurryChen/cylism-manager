import { api } from './index.js'

const segment = value => encodeURIComponent(String(value))
export const getProducts = options => api.get('/products', options)
export const getProduct = (id, options) => api.get(`/products/${segment(id)}`, options)
export const getProductResourceOptions = options => api.get('/products/resource-options', options)
export const createProduct = payload => api.post('/products', payload)
export const updateProduct = (id, payload) => api.put(`/products/${segment(id)}`, payload)
export const deleteProduct = id => api.delete(`/products/${segment(id)}`)
export const clearProducts = confirmation => api.post('/products/clear', { confirmation })

export const getProductPage = (query, options) => api.get('/products?' + new URLSearchParams(query), options)

export const getProjectWorkspace = (id, options) => api.get('/projects/' + segment(id) + '/workspace', options)
export const deleteProjectWorkspace = id => api.delete('/projects/' + segment(id) + '/workspace')
