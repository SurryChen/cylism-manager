import { api } from './index.js'

const base = '/cloud/static-assets'

export const getStaticAssetSettings = () => api.get(base + '/settings')
export const setStaticAssetSettings = settings => api.put(base + '/settings', settings)

export async function uploadStaticImage(file) {
  const { upload_url, fields, ticket } = await api.post(base + '/uploads', {
    content_type: file.type,
    size: file.size,
  })
  const body = new FormData()
  for (const [key, value] of Object.entries(fields)) body.append(key, value)
  body.append('file', file)
  const response = await fetch(upload_url, {
    method: 'POST',
    body,
    credentials: 'omit',
    redirect: 'error',
  }).catch(() => { throw new Error('无法直传 OSS，请检查网络及存储桶跨域配置') })
  if (!response.ok) throw new Error(`OSS 上传失败（HTTP ${response.status}）`)
  const { url } = await api.post(base + '/uploads/publish', { ticket })
  return url
}
