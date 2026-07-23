const API_BASE = '/api'

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

// 请求拦截
async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...options.headers }

  const token = getAccessToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  let res = await fetch(API_BASE + path, { ...options, headers })

  // 401 时尝试刷新 token
  if (res.status === 401 && getRefreshToken()) {
    const refreshRes = await fetch(API_BASE + '/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: getRefreshToken() })
    })
    if (refreshRes.ok) {
      const data = await refreshRes.json()
      setTokens(data.access_token, data.refresh_token)
      headers['Authorization'] = `Bearer ${data.access_token}`
      res = await fetch(API_BASE + path, { ...options, headers })
    } else {
      clearTokens()
      window.location.hash = '#/login'
      throw new Error('Token expired')
    }
  }

  return res
}

// API 方法
export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: 'POST', body: JSON.stringify(body) }),
  put: (path, body) => request(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (path) => request(path, { method: 'DELETE' }),
}

export { getAccessToken, getRefreshToken, setTokens, clearTokens }
