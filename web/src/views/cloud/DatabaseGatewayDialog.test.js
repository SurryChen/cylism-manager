import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseGatewayDialog from './DatabaseGatewayDialog.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const api = vi.hoisted(() => ({ getDatabaseGateway: vi.fn(), configureDatabaseGateway: vi.fn(), runDatabaseGatewayAction: vi.fn(), getDatabaseGatewayTaskLogs: vi.fn() }))
vi.mock('../../api/databases.js', () => api)
vi.mock('../../api/servers.js', () => ({ getServers: vi.fn().mockResolvedValue([{ id: 7, name: 'ECS', host: '192.0.2.1' }]) }))
vi.mock('../../api/clusters.js', () => ({ getClusters: vi.fn().mockResolvedValue([{ id: 2, name: '应用集群', status: 'ready' }]) }))

const instance = { id: 13, name: 'rds', host: 'rds.internal', port: 3306 }
const config = { id: 1, instance_id: 13, server_id: 7, cluster_id: 2, listen_ip: '100.80.0.1', listen_port: 13306, probe_image: 'python:3.12-alpine', allowed_sources: ['100.80.0.2/32'], status: 'ready' }
const activeView = () => ({ gateway: { ...config }, tasks: [{ id: 5, kind: 'deploy', status: 'succeeded', progress: '各节点 3/3 次通过' }], service_host: 'cylism-db-gateway-1.cylism-databases.svc.cluster.local', tls_server_name: 'rds.internal' })
const wrappers = []
function render() { const wrapper = mount(DatabaseGatewayDialog, { props: { instance }, global: { stubs: { Teleport: true } } }); wrappers.push(wrapper); return wrapper }

beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks()
  api.getDatabaseGateway.mockResolvedValue({ gateway: null, tasks: [] })
  api.getDatabaseGatewayTaskLogs.mockResolvedValue([])
  api.configureDatabaseGateway.mockResolvedValue(config)
  api.runDatabaseGatewayAction.mockResolvedValue({ id: 6, status: 'queued' })
})
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })

describe('private database gateway', () => {
  it('submits explicit private listener and source allowlist without database credentials', async () => {
    const wrapper = render(); await flushPromises()
    const menus = wrapper.findAllComponents(SelectMenu)
    menus[0].vm.$emit('update:modelValue', 7)
    menus[1].vm.$emit('update:modelValue', 2)
    await wrapper.get('[data-testid=gateway-listen-ip]').setValue('100.80.0.1')
    await wrapper.get('[data-testid=gateway-sources]').setValue('100.80.0.2/32\n10.42.0.0/24')
    await wrapper.get('[data-testid=gateway-save]').trigger('click'); await flushPromises()
    expect(api.configureDatabaseGateway).toHaveBeenCalledWith(13, { server_id: 7, cluster_id: 2, listen_ip: '100.80.0.1', listen_port: 13306, allowed_sources: ['100.80.0.2/32', '10.42.0.0/24'], probe_image: 'python:3.12-alpine' })
    expect(wrapper.text()).toContain('只验证网络，不验证账号认证或 TLS')
  })

  it('blocks concurrent mutations while a persisted task is running and shows logs', async () => {
    const view = activeView(); view.tasks[0].status = 'running'
    api.getDatabaseGateway.mockResolvedValue(view)
    api.getDatabaseGatewayTaskLogs.mockResolvedValue([{ id: 1, created_at: '2026-11-06T00:00:00Z', message: '节点 node-a：3/3 次通过' }])
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('[data-testid=gateway-save]').element.disabled).toBe(true)
    expect(wrapper.get('[data-testid=gateway-disable]').element.disabled).toBe(true)
    expect(wrapper.get('[data-testid=gateway-check]').element.disabled).toBe(true)
    expect(wrapper.text()).toContain('节点 node-a：3/3 次通过')
    expect(api.getDatabaseGatewayTaskLogs).toHaveBeenCalledWith(13, 5)
  })

  it('refreshes progress without overwriting unsaved edits and stops polling on close', async () => {
    api.getDatabaseGateway.mockResolvedValue(activeView())
    const wrapper = render(); await flushPromises()
    await wrapper.get('[data-testid=gateway-sources]').setValue('100.80.0.9/32')
    await vi.advanceTimersByTimeAsync(2500); await flushPromises()
    expect(wrapper.get('[data-testid=gateway-sources]').element.value).toBe('100.80.0.9/32')
    expect(wrapper.get('[data-testid=gateway-listen-ip]').element.disabled).toBe(true)
    await wrapper.setProps({ instance: null }); await flushPromises()
    const calls = api.getDatabaseGateway.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(api.getDatabaseGateway).toHaveBeenCalledTimes(calls)
  })

  it('requires confirmation before disabling a gateway', async () => {
    api.getDatabaseGateway.mockResolvedValue(activeView())
    const wrapper = render(); await flushPromises()
    await wrapper.get('[data-testid=gateway-disable]').trigger('click')
    expect(api.runDatabaseGatewayAction).not.toHaveBeenCalled()
    await wrapper.get('[data-testid=confirm-dialog-confirm]').trigger('click'); await flushPromises()
    expect(api.runDatabaseGatewayAction).toHaveBeenCalledWith(13, 'disable')
  })

  it('ignores an older poll after reopening the same instance', async () => {
    api.getDatabaseGateway.mockResolvedValue(activeView())
    const wrapper = render(); await flushPromises()
    let resolveOlder
    api.getDatabaseGateway.mockImplementationOnce(() => new Promise(resolve => { resolveOlder = resolve }))
    await vi.advanceTimersByTimeAsync(2500)
    await wrapper.setProps({ instance: null }); await flushPromises()
    await wrapper.setProps({ instance }); await flushPromises()
    const older = activeView()
    older.gateway.status = 'error'
    older.gateway.last_error = '过期的检查结果'
    resolveOlder(older); await flushPromises()
    expect(wrapper.text()).not.toContain('过期的检查结果')
    expect(wrapper.text()).toContain('网络已验证')
  })

  it('prevents saving when the existing gateway could not be read', async () => {
    api.getDatabaseGateway.mockRejectedValue(new Error('Agent 不可用'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('Agent 不可用')
    expect(wrapper.get('[data-testid=gateway-save]').element.disabled).toBe(true)
    expect(api.configureDatabaseGateway).not.toHaveBeenCalled()
  })
})
