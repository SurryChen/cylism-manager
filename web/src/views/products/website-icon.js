import { webURL } from './product-utils.js'

const iconPaths = ['/favicon.ico', '/assets/app-icon.svg', '/favicon.svg', '/favicon.png', '/apple-touch-icon.png']

function loadIcon(url, signal) {
  return new Promise((resolve, reject) => {
    const image = new Image()
    function finish(loaded) {
      clearTimeout(timeout)
      signal.removeEventListener('abort', abort)
      image.onload = image.onerror = null
      if (!loaded) image.removeAttribute('src')
      if (loaded) resolve(url)
      else reject(new Error('图标无法加载'))
    }
    const abort = () => finish(false)
    const timeout = setTimeout(abort, 4000)
    signal.addEventListener('abort', abort, { once: true })
    image.referrerPolicy = 'no-referrer'
    image.onload = () => finish(image.naturalWidth > 0)
    image.onerror = abort
    image.src = url
  })
}

export async function findWebsiteIcon({ signal }, address) {
  const input = address.trim()
  const site = webURL(input.includes('://') ? input : `https://${input}`)
  if (!site || !/^[^\s\\]+$/.test(input)) throw new Error('请输入有效的网站地址，例如 https://example.com')
  signal.throwIfAborted()
  const controller = new AbortController()
  const cancel = () => controller.abort()
  signal.addEventListener('abort', cancel, { once: true })
  try {
    return await Promise.any(iconPaths.map(path => loadIcon(site.origin + path, controller.signal)))
  } catch {
    throw new Error('未找到可用的网站图标，可以手动填写图标地址。')
  } finally {
    signal.removeEventListener('abort', cancel)
    cancel()
  }
}
