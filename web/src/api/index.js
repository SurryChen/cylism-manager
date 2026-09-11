const API_BASE = '/api'
let refreshPromise = null

// Token 管理
function getAccessToken() { return localStorage.getItem('access_token') }
function getRefreshToken() { return localStorage.getItem('refresh_token') }
function setTokens(access, refresh) {
  localStorage.setItem('access_token', access)
  localStorage.setItem('refresh_token', refresh)
}
function clearTokens() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
}

async function refreshAccessToken() {
  if (refreshPromise) return refreshPromise
  const refreshToken = getRefreshToken()
  if (!refreshToken) throw new Error('Token expired')

  refreshPromise = (async () => {
    const refreshRes = await fetch(API_BASE + '/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!refreshRes.ok) throw new Error('Token expired')
    const data = await unwrapResponse(refreshRes)
    if (!data?.access_token || !data?.refresh_token) throw new Error('Token expired')
    setTokens(data.access_token, data.refresh_token)
    return data.access_token
  })()

  try {
    return await refreshPromise
  } catch (error) {
    if (error?.name === 'AbortError') throw error
    clearTokens()
    window.location.hash = '#/login'
    throw error instanceof Error ? error : new Error('Token expired')
  } finally {
    refreshPromise = null
  }
}

function awaitWithAbort(promise, signal) {
  if (!signal) return promise
  if (signal.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
  return new Promise((resolve, reject) => {
    const onAbort = () => reject(new DOMException('Aborted', 'AbortError'))
    signal.addEventListener('abort', onAbort, { once: true })
    promise.then(resolve, reject).finally(() => signal.removeEventListener('abort', onAbort))
  })
}

// 统一响应解包
async function unwrapResponse(res) {
  const raw = await res.text()
  let json
  try {
    json = JSON.parse(raw)
  } catch {
    if (res.headers.get('content-type')?.includes('text/html') || /^\s*</.test(raw)) {
      throw new Error('服务端未提供此 API（开发代理可能指向了旧服务，请检查 VITE_API_PROXY_TARGET）')
    }
    throw new Error(`服务端返回了无效 JSON（HTTP ${res.status}）`)
  }
  if (json.code !== 0) {
    throw new Error(json.message || 'unknown error')
  }
  return json.data
}

// 请求拦截
async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...options.headers }

  const token = getAccessToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  let res = await fetch(API_BASE + path, { ...options, headers })

  // 401 时尝试刷新 token；并发请求共享同一个 refresh Promise。
  if (res.status === 401 && getRefreshToken()) {
    const accessToken = await awaitWithAbort(refreshAccessToken(), options.signal)
    headers['Authorization'] = `Bearer ${accessToken}`
    res = await fetch(API_BASE + path, { ...options, headers })
  }

  // 统一解包
  const data = await unwrapResponse(res)
  return data
}

// API 方法 — 全部自动解包，调用方直接拿到 data
export const api = {
  get: (path, options = {}) => request(path, options),
  post: (path, body, options = {}) => request(path, { ...options, method: 'POST', body: JSON.stringify(body) }),
  put: (path, body, options = {}) => request(path, { ...options, method: 'PUT', body: JSON.stringify(body) }),
  patch: (path, body, options = {}) => request(path, { ...options, method: 'PATCH', body: JSON.stringify(body) }),
  delete: (path, body, options = {}) => request(path, { ...options, method: 'DELETE', ...(body === undefined ? {} : { body: JSON.stringify(body) }) }),
}

export { getAccessToken, getRefreshToken, setTokens, clearTokens }
