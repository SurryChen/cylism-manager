import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Databases from './Databases.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const api = vi.hoisted(() => ({ getDatabases: vi.fn(), getMySQLClusters: vi.fn(), createDatabase: vi.fn(), updateDatabaseConnection: vi.fn(), getDatabaseBindings: vi.fn(), getDatabaseApplications: vi.fn(), bindDatabase: vi.fn(), getDatabaseReplications: vi.fn() }))
vi.mock('../../api/databases.js', async importOriginal => ({ ...await importOriginal(), ...api }))
vi.mock('../../api/applications.js', () => ({ getApplications: vi.fn().mockResolvedValue([{ id: 13, name: 'app' }]), getDeploymentTemplates: vi.fn().mockResolvedValue([{ id: 101, name: '默认模板' }]) }))
vi.mock('../../api/clusters.js', () => ({ getClusters: vi.fn().mockResolvedValue([]) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

const endpoint = { id: 1, name: 'RDS', mode: 'external', connection_mode: 'gateway', host: 'rds.internal', port: 3306, status: 'registered', database_name: '', username: '' }
const wrappers = []
const button = (wrapper, text) => wrapper.findAll('button').find(item => item.text() === text)
async function render(instance = endpoint) {
  api.getDatabases.mockResolvedValue([{ ...instance }])
  const wrapper = mount(Databases, { global: { stubs: { Teleport: true, DatabaseGatewayDialog: { props: ['instance'], template: '<div v-if="instance" data-testid="gateway-dialog">网关配置 · {{ instance.host }}</div>' } } } })
  wrappers.push(wrapper)
  await flushPromises()
  await button(wrapper, '已有实例').trigger('click')
  return wrapper
}
async function openApplications(wrapper) {
  await button(wrapper, '查看与配置').trigger('click'); await flushPromises()
  await button(wrapper, '应用').trigger('click')
}
beforeEach(() => {
  vi.clearAllMocks()
  api.getMySQLClusters.mockResolvedValue([])
  api.createDatabase.mockResolvedValue({ ...endpoint })
  api.updateDatabaseConnection.mockResolvedValue({ ...endpoint, database_name: 'app_db', username: 'app_user' })
  api.getDatabaseBindings.mockResolvedValue([])
  api.bindDatabase.mockResolvedValue({})
  api.getDatabaseReplications.mockResolvedValue([])
  api.getDatabaseApplications.mockResolvedValue({ items: [], warnings: [] })
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('external MySQL registration and business connection', () => {
  it('registers an endpoint and opens its gateway without asking for business credentials', async () => {
    const wrapper = await render()
    await button(wrapper, '接入已有 MySQL').trigger('click')
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
    await wrapper.get('[data-testid=instance-name]').setValue('RDS')
    await wrapper.get('[data-testid=instance-host]').setValue('rds.internal')
    const menu = wrapper.findAllComponents(SelectMenu).find(item => item.props('ariaLabel') === '连接方式')
    menu.vm.$emit('update:modelValue', 'gateway')
    await flushPromises()
    await button(wrapper, '下一步：配置网关').trigger('click'); await flushPromises()
    expect(api.createDatabase).toHaveBeenCalledWith({ name: 'RDS', mode: 'external', host: 'rds.internal', port: 3306, connection_mode: 'gateway' })
    expect(wrapper.get('[data-testid=gateway-dialog]').text()).toContain('rds.internal')
  })

  it('guides an unconfigured instance to business connection instead of allowing an empty binding', async () => {
    const wrapper = await render()
    await openApplications(wrapper)
    expect(wrapper.text()).toContain('未配置业务连接')
    await button(wrapper, '应用绑定').trigger('click'); await flushPromises()
    expect(api.getDatabaseBindings).not.toHaveBeenCalled()
    expect(button(wrapper, '绑定到模板')).toBeUndefined()
    expect(wrapper.text()).toContain('私网网关可以先独立部署和验证')
    await button(wrapper, '配置业务连接').trigger('click')
    expect(wrapper.get('[data-testid=connection-password]').element.value).toBe('')
    expect(api.bindDatabase).not.toHaveBeenCalled()
  })

  it('saves existing credentials separately, refreshes the row and clears the password on reopen', async () => {
    const wrapper = await render()
    await openApplications(wrapper)
    await button(wrapper, '业务连接').trigger('click')
    await wrapper.get('[data-testid=connection-database]').setValue('app_db')
    await wrapper.get('[data-testid=connection-username]').setValue('app_user')
    await wrapper.get('[data-testid=connection-password]').setValue('existing-password')
    api.getDatabases.mockResolvedValue([{ ...endpoint, database_name: 'app_db', username: 'app_user' }])
    await wrapper.get('[data-testid=connection-save]').trigger('click'); await flushPromises()
    expect(api.updateDatabaseConnection).toHaveBeenCalledWith(1, { database_name: 'app_db', username: 'app_user', password: 'existing-password' })
    expect(api.createDatabase).not.toHaveBeenCalled()
    await openApplications(wrapper)
    expect(wrapper.text()).toContain('app_db')
    await button(wrapper, '业务连接').trigger('click')
    expect(wrapper.get('[data-testid=connection-password]').element.value).toBe('')
    await wrapper.get('[data-testid=connection-password]').setValue('discarded-password')
    await button(wrapper, '取消').trigger('click')
    await openApplications(wrapper)
    await button(wrapper, '业务连接').trigger('click')
    expect(wrapper.get('[data-testid=connection-password]').element.value).toBe('')
  })

  it('keeps existing configured instances bindable and surfaces credential save errors', async () => {
    const wrapper = await render({ ...endpoint, database_name: 'app_db', username: 'app_user' })
    await openApplications(wrapper)
    await button(wrapper, '应用绑定').trigger('click'); await flushPromises()
    expect(api.getDatabaseBindings).toHaveBeenCalledWith(1)
    expect(button(wrapper, '绑定到模板')).toBeDefined()
    await button(wrapper, '关闭').trigger('click')
    await openApplications(wrapper)
    await button(wrapper, '业务连接').trigger('click')
    await wrapper.get('[data-testid=connection-password]').setValue('existing-password')
    api.updateDatabaseConnection.mockRejectedValue(new Error('业务连接仍被上线模板使用，请先解除绑定'))
    await wrapper.get('[data-testid=connection-save]').trigger('click'); await flushPromises()
    expect(wrapper.get('[role=alert]').text()).toContain('请先解除绑定')
    expect(wrapper.find('[data-testid=connection-password]').exists()).toBe(true)
  })
})

it('keeps gateway diagnostic failures separate from the registered database status', async () => {
  const wrapper = await render({ ...endpoint, private_gateway: { status: 'error', last_error: 'node-a denied', cluster_id: 2 } })
  expect(wrapper.text()).toContain('已登记')
  expect(wrapper.text()).not.toContain('异常')
  await button(wrapper, '查看与配置').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('登录终端')
  expect(wrapper.text()).toContain('资源监控')
  expect(wrapper.text()).toContain('查看诊断')
  expect(wrapper.find('.detail-tabs .tab-btn').exists()).toBe(false)
  expect(wrapper.find('.section-tabs-header--compact .section-tab').exists()).toBe(true)
  await button(wrapper, '查看诊断').trigger('click')
  expect(wrapper.get('[role=alert]').text()).toContain('node-a denied')
})

it('offers discovery without claiming that an unregistered replica does not exist', async () => {
  const wrapper = await render()
  await button(wrapper, '查看与配置').trigger('click'); await flushPromises()
  await button(wrapper, '复制关系').trigger('click')
  expect(wrapper.text()).toContain('尚未登记复制关系')
  expect(wrapper.text()).toContain('不代表数据库没有运行复制')
  await button(wrapper, '发现已有复制').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('不会初始化数据或修改复制通道')
  expect(wrapper.findAll('input[type=password]')).toHaveLength(0)
  expect(wrapper.getComponent({ name: 'DatabaseCapabilityDialog' }).props('source')).toBe('instance:1')
})
