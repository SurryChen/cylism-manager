import { api, getAccessToken } from './index.js'

export function getRuntimes(options) { return api.get('/runtimes', options) }
export function getRuntimeCatalog(options) { return api.get('/runtimes/catalog', options) }
export function getRuntimeNodes(options) { return api.get('/nodes', options) }
export function getRuntimeNamespaces(options) { return api.get('/k8s/namespace-names', options) }
export function getRuntimeAgentState(runtimeID, options) {
  return Promise.all([
    api.get(`/runtimes/${runtimeID}/agent-capability-grants`, options),
    api.get(`/runtimes/${runtimeID}/agent-operations`, options),
  ])
}

function post(path, body, options) {
  if (body === undefined) return options === undefined ? api.post(path) : api.post(path, undefined, options)
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

function put(path, body, options) {
  return options === undefined ? api.put(path, body) : api.put(path, body, options)
}

export function createRuntime(body, options) {
  return post('/runtimes', body, options)
}

export function updateRuntime(runtimeID, body, options) {
  return put(`/runtimes/${runtimeID}`, body, options)
}

export function deployRuntime(runtimeID, options) {
  return post(`/runtimes/${runtimeID}/deploy`, undefined, options)
}

export function healthCheckRuntime(runtimeID, options) {
  return post(`/runtimes/${runtimeID}/health-check`, undefined, options)
}

export function uninstallRuntime(runtimeID, { deleteData = false } = {}, options) {
  return post(`/runtimes/${runtimeID}/uninstall${deleteData ? '?delete_data=true' : ''}`, undefined, options)
}

export function installRuntimeAgentTools(runtimeID, options) {
  return post(`/runtimes/${runtimeID}/agent-tools/install`, undefined, options)
}

export function updateRuntimeAgentTools(runtimeID, options) {
  return post(`/runtimes/${runtimeID}/agent-tools/update`, undefined, options)
}

export function uninstallRuntimeAgentTools(runtimeID, options) {
  return post(`/runtimes/${runtimeID}/agent-tools/uninstall`, undefined, options)
}

export function agentCapabilityGrants(runtimeID, options) {
  return api.get(`/runtimes/${runtimeID}/agent-capability-grants`, options)
}

export function saveAgentCapabilityGrants(runtimeID, grants, options) {
  return put(`/runtimes/${runtimeID}/agent-capability-grants`, { grants }, options)
}

export function agentOperations(runtimeID, { status, sessionID } = {}, options) {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (sessionID) params.set('session_id', sessionID)
  const query = params.toString()
  return api.get(`/runtimes/${runtimeID}/agent-operations${query ? `?${query}` : ''}`, options)
}

export function resolveAgentOperation(operationID, approve, options) {
  return post(`/agent-operations/${encodeURIComponent(operationID)}/${approve ? 'approve' : 'reject'}`, undefined, options)
}

export function chatSessions(runtimeID, { archived = false } = {}, options) {
  return api.get(`/runtimes/${runtimeID}/chat/sessions${archived ? '?archived=true' : ''}`, options)
}

export function chatMessages(runtimeID, sessionID, { limit, before } = {}, options) {
  const params = new URLSearchParams()
  if (Number.isInteger(limit) && limit > 0) params.set('limit', String(limit))
  if (before) params.set('before', before)
  const query = params.toString()
  return api.get(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/messages${query ? `?${query}` : ''}`, options)
}

export function renameChatSession(runtimeID, sessionID, title, options) {
  return options === undefined
    ? api.patch(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}`, { title })
    : api.patch(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}`, { title }, options)
}

export function archiveChatSession(runtimeID, sessionID, archived, options) {
  return post(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/${archived ? 'archive' : 'restore'}`, undefined, options)
}

export function exportChatSession(runtimeID, sessionID, options) {
  return api.get(`/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}/export`, options)
}

export function deleteChatSession(runtimeID, sessionID, options) {
  const path = `/runtimes/${runtimeID}/chat/sessions/${encodeURIComponent(sessionID)}`
  return options === undefined ? api.delete(path) : api.delete(path, undefined, options)
}

// Sends a chat message and parses the SSE response. The returned promise has
// an abort method so callers can stop a stream during navigation or unmount.
export function chatStream(runtimeID, body, { onEvent, signal } = {}) {
  const controller = new AbortController()
  const abort = () => controller.abort()
  const promise = (async () => {
    const headers = { 'Content-Type': 'application/json' }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    const res = await fetch(`/api/runtimes/${runtimeID}/chat`, {
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
