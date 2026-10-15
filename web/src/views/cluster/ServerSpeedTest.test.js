import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ServerSpeedTest from './ServerSpeedTest.vue'
import { getServerSpeedDependencies, installServerSpeedDependencies, runServerSpeedTest } from '../../api/servers.js'

vi.mock('../../api/servers.js', () => ({
  getServerSpeedDependencies: vi.fn(),
  installServerSpeedDependencies: vi.fn(),
  runServerSpeedTest: vi.fn(),
}))

const servers = [
  { id: 1, name: '来源', host: '192.0.2.1' },
  { id: 2, name: '目标', host: '192.0.2.2' },
]

beforeEach(() => {
  vi.resetAllMocks()
  sessionStorage.removeItem('cylism.server-speed-test.results.v1')
  getServerSpeedDependencies.mockImplementation(id => Promise.resolve({ server_id: Number(id), iperf3_installed: true, timeout_installed: true }))
})

describe('ServerSpeedTest', () => {
  it('keeps multiple results in newest-first order and displays throughput in MB/s', async () => {
    runServerSpeedTest.mockResolvedValueOnce({
      source_id: 1, target_id: 2, target_address: '100.64.0.2', address: 'tailscale',
      protocol: 'udp', direction: 'reverse', sender_mbps: 1, receiver_mbps: 0.724,
      lost_packets: 692, total_packets: 2501, loss_percent: 27.67,
    }).mockResolvedValueOnce({
      source_id: 1, target_id: 2, target_address: '192.0.2.2', address: 'registered',
      protocol: 'tcp', direction: 'forward', sender_mbps: 47.91, receiver_mbps: 45.29,
      retransmits: 0,
    })
    const wrapper = mount(ServerSpeedTest, { props: { servers } })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('1')
    await selects[1].setValue('2')
    await selects[2].setValue('tailscale')
    await selects[3].setValue('udp')
    await selects[4].setValue('reverse')
    expect(getServerSpeedDependencies).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('环境检查')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(getServerSpeedDependencies).toHaveBeenCalledTimes(2)
    expect(runServerSpeedTest).toHaveBeenCalledWith({ source_id: 1, target_id: 2, address: 'tailscale', protocol: 'udp', direction: 'reverse' })
    expect(wrapper.text()).toContain('0.091 MB/s')
    expect(wrapper.text()).toContain('0.125 MB/s')
    expect(wrapper.text()).toContain('692 / 2501 (27.7%)')
    await selects[3].setValue('tcp')
    expect(wrapper.text()).toContain('0.091 MB/s')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    const rows = wrapper.findAll('.speed-results-table tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('5.66 MB/s')
    expect(rows[1].text()).toContain('0.091 MB/s')
    expect(wrapper.text()).toContain('2 条记录')
    expect(JSON.parse(sessionStorage.getItem('cylism.server-speed-test.results.v1'))).toHaveLength(2)
    wrapper.unmount()

    const restored = mount(ServerSpeedTest, { props: { servers } })
    expect(restored.findAll('.speed-results-table tbody tr')).toHaveLength(2)
    restored.unmount()
  })

  it('prompts for missing tools only after testing starts, then installs and tests', async () => {
    getServerSpeedDependencies.mockImplementation(id => Promise.resolve({ server_id: Number(id), iperf3_installed: Number(id) !== 1, timeout_installed: Number(id) !== 2 }))
    installServerSpeedDependencies.mockImplementation(id => Promise.resolve({ server_id: Number(id), iperf3_installed: true, timeout_installed: true }))
    runServerSpeedTest.mockResolvedValue({ source_id: 1, target_id: 2, target_address: '192.0.2.2', address: 'registered', protocol: 'tcp', direction: 'forward', sender_mbps: 10, receiver_mbps: 9, retransmits: 0 })
    const wrapper = mount(ServerSpeedTest, { props: { servers } })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('1')
    await selects[1].setValue('2')
    expect(getServerSpeedDependencies).not.toHaveBeenCalled()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[role="dialog"]').text()).toContain('来源')
    expect(wrapper.get('[role="dialog"]').text()).toContain('iperf3')
    expect(wrapper.get('[role="dialog"]').text()).toContain('timeout')
    expect(runServerSpeedTest).not.toHaveBeenCalled()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[role="dialog"] .modal-actions .btn:first-child').trigger('click')
    expect(installServerSpeedDependencies).not.toHaveBeenCalled()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(installServerSpeedDependencies).toHaveBeenCalledWith(1)
    expect(installServerSpeedDependencies).toHaveBeenCalledWith(2)
    expect(runServerSpeedTest).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('1.13 MB/s')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows installation progress while the request is pending', async () => {
    getServerSpeedDependencies.mockImplementation(id => Promise.resolve({ server_id: Number(id), iperf3_installed: Number(id) !== 1, timeout_installed: true }))
    runServerSpeedTest.mockResolvedValue({ source_id: 1, target_id: 2, target_address: '192.0.2.2', address: 'registered', protocol: 'tcp', direction: 'forward', sender_mbps: 10, receiver_mbps: 9, retransmits: 0 })
    let finishInstall
    installServerSpeedDependencies.mockImplementation(() => new Promise(resolve => { finishInstall = resolve }))
    const wrapper = mount(ServerSpeedTest, { props: { servers } })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('1')
    await selects[1].setValue('2')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('正在安装 来源 · 00:00')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    finishInstall({ server_id: 1, iperf3_installed: true, timeout_installed: true })
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the install prompt open when installation fails', async () => {
    getServerSpeedDependencies.mockImplementation(id => Promise.resolve({ server_id: Number(id), iperf3_installed: Number(id) !== 1, timeout_installed: true }))
    installServerSpeedDependencies.mockRejectedValue(new Error('软件源不可用'))
    const wrapper = mount(ServerSpeedTest, { props: { servers } })
    const selects = wrapper.findAll('select')
    await selects[0].setValue('1')
    await selects[1].setValue('2')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[role="dialog"]').text()).toContain('软件源不可用')
    expect(runServerSpeedTest).not.toHaveBeenCalled()
    expect(wrapper.get('[role="dialog"] .btn-primary').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
})
