import { afterEach, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { uploadStaticImage } from './static-assets.js'

vi.mock('./index.js', () => ({ api: { post: vi.fn() } }))
afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetAllMocks()
})

it('sends image bytes to OSS and only sends metadata and ticket to Manager', async () => {
  const file = new File(['test'], 'icon.png', { type: 'image/png' })
  const uploadURL = 'https://bucket.example/pending/icon.png?signature=test'
  const headers = { 'Content-Type': 'image/png' }
  const publicURL = 'https://bucket.example/images/icon.png'
  api.post.mockResolvedValueOnce({ upload_url: uploadURL, headers, ticket: 'test-ticket' }).mockResolvedValueOnce({ url: publicURL })
  const directUpload = vi.fn().mockResolvedValue({ ok: true })
  vi.stubGlobal('fetch', directUpload)

  await expect(uploadStaticImage(file)).resolves.toBe(publicURL)
  expect(api.post.mock.calls).toEqual([
    ['/cloud/static-assets/uploads', { content_type: file.type, size: file.size }],
    ['/cloud/static-assets/uploads/publish', { ticket: 'test-ticket' }],
  ])
  expect(directUpload).toHaveBeenCalledExactlyOnceWith(uploadURL, {
    method: 'PUT', headers, body: file, credentials: 'omit', redirect: 'error',
  })
})
