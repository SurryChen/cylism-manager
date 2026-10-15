import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ServerSingBox from './ServerSingBox.vue'
import { getSingBoxStatus, importSingBoxConfig, installSingBox, startSingBox, stopSingBox } from '../../api/servers.js'

vi.mock('../../api/servers.js', () => ({
  getSingBoxStatus: vi.fn(), importSingBoxConfig: vi.fn(), installSingBox: vi.fn(),
  startSingBox: vi.fn(), stopSingBox: vi.fn(),
}))

let wrapper
const server = { id: 7, name: 'node-7', host: '192.0.2.7' }

beforeEach(() => {
  document.body.innerHTML = ''
  vi.resetAllMocks()
  getSingBoxStatus.mockResolvedValue({ server_id: 7, installed: true, configured: false, running: false, version: '1.12.0' })
})
afterEach(() => { wrapper?.unmount(); wrapper = null })

describe('ServerSingBox', () => {
  it('shows server status and imports a URL before allowing start', async () => {
    importSingBoxConfig.mockResolvedValue({ server_id: 7, installed: true, configured: true, running: false })
    startSingBox.mockResolvedValue({ server_id: 7, installed: true, configured: true, running: true })
    wrapper = mount(ServerSingBox, { props: { servers: [server] } })
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('1.12.0')
    await wrapper.get('button[aria-label="管理 node-7 的代理"]').trigger('click')
    expect(document.body.textContent).not.toContain('启动代理')
    const input = document.body.querySelector('#sing-box-url')
    input.value = 'https://example.com/config.json?token=secret'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.body.querySelector('.sing-box-form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(importSingBoxConfig).toHaveBeenCalledWith(7, 'https://example.com/config.json?token=secret')
    expect(input.value).toBe('')
    const start = [...document.body.querySelectorAll('button')].find(button => button.textContent === '启动代理')
    start.click()
    await flushPromises()
    expect(startSingBox).toHaveBeenCalledWith(7)
    expect(wrapper.get('tbody').text()).toContain('运行中')
  })

  it('requires confirmation before stopping', async () => {
    getSingBoxStatus.mockResolvedValue({ server_id: 7, installed: true, configured: true, running: true })
    stopSingBox.mockResolvedValue({ server_id: 7, installed: true, configured: true, running: false })
    wrapper = mount(ServerSingBox, { props: { servers: [server] } })
    await flushPromises()
    await wrapper.get('button[aria-label="管理 node-7 的代理"]').trigger('click')
    const stop = [...document.body.querySelectorAll('button')].find(button => button.textContent === '停止代理')
    stop.click()
    await flushPromises()
    expect(stopSingBox).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(stopSingBox).toHaveBeenCalledWith(7)
  })

  it('offers installation when sing-box is missing', async () => {
    getSingBoxStatus.mockResolvedValue({ server_id: 7, installed: false, configured: false, running: false })
    installSingBox.mockResolvedValue({ server_id: 7, installed: true, configured: false, running: false })
    wrapper = mount(ServerSingBox, { props: { servers: [server] } })
    await flushPromises()
    await wrapper.get('button[aria-label="管理 node-7 的代理"]').trigger('click')
    const install = [...document.body.querySelectorAll('button')].find(button => button.textContent === '安装 sing-box')
    install.click()
    await flushPromises()
    expect(installSingBox).toHaveBeenCalledWith(7, null)
    expect(document.body.querySelector('#sing-box-url')).toBeTruthy()
  })

  it('passes the selected direct server as installation egress', async () => {
    getSingBoxStatus.mockResolvedValue({ server_id: 7, installed: false, configured: false, running: false })
    installSingBox.mockResolvedValue({ server_id: 7, installed: true, configured: false, running: false })
    wrapper = mount(ServerSingBox, { props: { servers: [server, { id: 8, name: '出口节点', host: '192.0.2.8' }] } })
    await flushPromises()
    await wrapper.get('button[aria-label="管理 node-7 的代理"]').trigger('click')
    const select = document.body.querySelector('.sing-box-manage select')
    select.value = '8'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    const install = [...document.body.querySelectorAll('button')].find(button => button.textContent === '安装 sing-box')
    install.click()
    await flushPromises()
    expect(installSingBox).toHaveBeenCalledWith(7, 8)
  })

  it('does not treat a failed status request as a stopped service', async () => {
    getSingBoxStatus.mockRejectedValue(new Error('接口不可用'))
    wrapper = mount(ServerSingBox, { props: { servers: [server] } })
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('无法检测')
    await wrapper.get('button[aria-label="管理 node-7 的代理"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).not.toContain('安装 sing-box')
    expect(document.body.textContent).toContain('接口不可用')
  })
})
