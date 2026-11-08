import { api } from './index.js'

const segment = value => encodeURIComponent(String(value))
export const getProducts = options => api.get('/products', options)
export const getProduct = (id, options) => api.get(`/products/${segment(id)}`, options)
export const getProductResourceOptions = options => api.get('/products/resource-options', options)
export const createProduct = payload => api.post('/products', payload)
export const updateProduct = (id, payload) => api.put(`/products/${segment(id)}`, payload)
