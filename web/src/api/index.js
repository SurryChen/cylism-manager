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

  // 401 时尝试刷新 token
  if (res.status === 401 && getRefreshToken()) {
    const refreshRes = await fetch(API_BASE + '/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: getRefreshToken() })
    })
    if (refreshRes.ok) {
      const data = await unwrapResponse(refreshRes)
      setTokens(data.access_token, data.refresh_token)
      headers['Authorization'] = `Bearer ${data.access_token}`
      res = await fetch(API_BASE + path, { ...options, headers })
    } else {
      clearTokens()
      window.location.hash = '#/login'
      throw new Error('Token expired')
    }
  }

  // 统一解包
  const data = await unwrapResponse(res)
  return data
}

// API 方法 — 全部自动解包，调用方直接拿到 data
export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: 'POST', body: JSON.stringify(body) }),
  put: (path, body) => request(path, { method: 'PUT', body: JSON.stringify(body) }),
  patch: (path, body) => request(path, { method: 'PATCH', body: JSON.stringify(body) }),
  delete: (path, body) => request(path, { method: 'DELETE', ...(body === undefined ? {} : { body: JSON.stringify(body) }) }),
}

export function chatSessions(runtimeID, { archived = false } = {}) {
  return api.get(`/runtimes/${runtimeID}/chat/sessions${archived ? '?archived=true' : ''}`)
}

export function chatMessages(runtimeID, sessionID, { limit, before } = {}) {
  const params = new URLSearchParams()
  if (Number.isInteger(limit) && limit > 0) params.set('limit', String(limit))
  if (before) params.set('before', before)
  const query = params.toString()
  return api.get(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/messages${query ? `?${query}` : ''}`)
}

export function agentCapabilityGrants(runtimeID) {
  return api.get(`/runtimes/${runtimeID}/agent-capability-grants`)
}

export function saveAgentCapabilityGrants(runtimeID, grants) {
  return api.put(`/runtimes/${runtimeID}/agent-capability-grants`, { grants })
}

export function agentOperations(runtimeID, { status, sessionID } = {}) {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (sessionID) params.set('session_id', sessionID)
  const query = params.toString()
  return api.get(`/runtimes/${runtimeID}/agent-operations${query ? `?${query}` : ''}`)
}

export function resolveAgentOperation(operationID, approve) {
  return api.post(`/agent-operations/${encodeURIComponent(operationID)}/${approve ? 'approve' : 'reject'}`)
}

export function renameChatSession(runtimeID, sessionID, title) {
  return api.patch(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}`, { title })
}

export function archiveChatSession(runtimeID, sessionID, archived) {
  return api.post(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/${archived ? 'archive' : 'restore'}`)
}

export function exportChatSession(runtimeID, sessionID) {
  return api.get(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/export`)
}

export function deleteChatSession(runtimeID, sessionID) {
  return api.delete(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}`)
}

// chatStream 发送聊天消息并解析 SSE 流。onEvent 收到 {type:'delta'|'done'|'error'}。
// 返回 abort 函数；点击停止或组件卸载时调用。
export function chatStream(runtimeID, body, { onEvent, signal } = {}) {
  const controller = new AbortController()
  const abort = () => controller.abort()
  const promise = (async () => {
    const headers = { 'Content-Type': 'application/json' }
    const token = getAccessToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
    const res = await fetch(API_BASE + `/runtimes/${runtimeID}/chat`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      signal: signal || controller.signal,
    })
    if (!res.ok || !res.body) {
      let message = `HTTP ${res.status}`
      try {
        const data = await res.json()
        message = data.message || message
      } catch { /* keep default */ }
      throw new Error(message)
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    const dispatch = () => {
      let index
      while ((index = buffer.indexOf('\n\n')) !== -1) {
        const rawEvent = buffer.slice(0, index)
        buffer = buffer.slice(index + 2)
        for (const line of rawEvent.split('\n')) {
          if (!line.startsWith('data:')) continue
          const payload = line.slice(5).trim()
          if (!payload) continue
          try { onEvent(JSON.parse(payload)) } catch { /* ignore malformed event */ }
        }
      }
    }
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      dispatch()
    }
    buffer += decoder.decode()
    dispatch()
  })()
  promise.abort = abort
  return promise
}

export { getAccessToken, getRefreshToken, setTokens, clearTokens }
