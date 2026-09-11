import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  agentOperations,
  chatMessages,
  chatSessions,
  createRuntime,
  deployRuntime,
  getRuntimeAgentState,
  getRuntimeCatalog,
  getRuntimes,
  healthCheckRuntime,
  installRuntimeAgentTools,
  renameChatSession,
  resolveAgentOperation,
  saveAgentCapabilityGrants,
  uninstallRuntime,
  updateRuntime,
} from './runtimes.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn() } }))

describe('runtimes api', () => {
  afterEach(() => vi.clearAllMocks())

  it('loads runtime resources with a shared signal', () => {
    const options = { signal: new AbortController().signal }
    getRuntimeCatalog(options)
    getRuntimes(options)
    getRuntimeAgentState(4, options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/runtimes/catalog', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/runtimes', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/runtimes/4/agent-capability-grants', options)
    expect(api.get).toHaveBeenNthCalledWith(4, '/runtimes/4/agent-operations', options)
  })

  it('owns runtime lifecycle and Agent mutations', () => {
    const options = { signal: new AbortController().signal }
    createRuntime({ name: 'main' }, options)
    updateRuntime(4, { name: 'updated' }, options)
    deployRuntime(4, options)
    healthCheckRuntime(4, options)
    uninstallRuntime(4, { deleteData: true }, options)
    installRuntimeAgentTools(4, options)
    saveAgentCapabilityGrants(4, [{ capability: 'cluster.read' }], options)
    resolveAgentOperation('op/1', true, options)
    expect(api.post).toHaveBeenCalledWith('/runtimes/4/deploy', undefined, options)
    expect(api.post).toHaveBeenCalledWith('/runtimes/4/health-check', undefined, options)
    expect(api.post).toHaveBeenCalledWith('/runtimes/4/uninstall?delete_data=true', undefined, options)
    expect(api.post).toHaveBeenCalledWith('/runtimes/4/agent-tools/install', undefined, options)
    expect(api.put).toHaveBeenCalledWith('/runtimes/4/agent-capability-grants', { grants: [{ capability: 'cluster.read' }] }, options)
    expect(api.post).toHaveBeenCalledWith('/agent-operations/op%2F1/approve', undefined, options)
    expect(api.post).toHaveBeenCalledWith('/runtimes', { name: 'main' }, options)
    expect(api.put).toHaveBeenCalledWith('/runtimes/4', { name: 'updated' }, options)
  })

  it('owns chat and approval reads with optional request options', () => {
    const options = { signal: new AbortController().signal }
    chatSessions(4, { archived: true }, options)
    chatMessages(4, 'session/1', { limit: 50, before: 'cursor' }, options)
    agentOperations(4, { status: 'pending approval', sessionID: 'session/1' }, options)
    renameChatSession(4, 'session/1', 'Renamed', options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/runtimes/4/chat/sessions?archived=true', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/runtimes/4/chat/sessions/session%2F1/messages?limit=50&before=cursor', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/runtimes/4/agent-operations?status=pending+approval&session_id=session%2F1', options)
    expect(api.patch).toHaveBeenCalledWith('/runtimes/4/chat/sessions/session%2F1', { title: 'Renamed' }, options)
  })
})
