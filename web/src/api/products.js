import { api } from './index.js'
import { uploadStaticImage } from './static-assets.js'

const segment = value => encodeURIComponent(String(value))
export const getProducts = options => api.get('/products', options)
export const getProduct = (id, options) => api.get(`/products/${segment(id)}`, options)
export const getProductResourceOptions = options => api.get('/products/resource-options', options)
export const createProduct = async payload => api.post('/products', await withIconURL(payload))
export const updateProduct = async (id, payload) => api.put(`/products/${segment(id)}`, await withIconURL(payload))
export const deleteProduct = id => api.delete(`/products/${segment(id)}`)
export const clearProducts = confirmation => api.post('/products/clear', { confirmation })

export const getProductPage = (query, options) => api.get('/products?' + new URLSearchParams(query), options)

export const getProjectWorkspace = (id, options) => api.get('/projects/' + segment(id) + '/workspace', options)
export const deleteProjectWorkspace = id => api.delete('/projects/' + segment(id) + '/workspace')

async function withIconURL({ iconFile, ...payload }) {
  if (iconFile) payload.icon_url = await uploadStaticImage(iconFile)
  return payload
}
