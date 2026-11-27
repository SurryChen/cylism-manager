import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WeChatChannelSettings from './WeChatChannelSettings.vue'

const mocks = vi.hoisted(() => ({ settings: vi.fn(), save: vi.fn(), sync: vi.fn() }))
vi.mock('../../api/wechat.js', async importOriginal => ({
  ...await importOriginal(),
  getWeChatSettings: mocks.settings,
  saveWeChatConnection: mocks.save,
  syncWeChatRelease: mocks.sync,
}))

const product = {
  id: 7,
  channels: [
    { id: 1, name: '小程序', platform: 'wechat_mini_program' },
    { id: 2, name: '小游戏', platform: 'mini_game' },
  ],
}
const channel = { channel_id: 2, app_id: 'wx1234567890abcdef', release_connected: true, analytics_connected: true }
const mountSettings = authorizationReturn => mount(WeChatChannelSettings, {
  props: { product, open: true, authorizationReturn },
  global: { stubs: { Teleport: true } },
})

beforeEach(() => {
  vi.clearAllMocks()
  mocks.settings.mockResolvedValue({ component_available: true, connections: [channel] })
  mocks.sync.mockResolvedValue({
    component_available: true,
    connections: [{ ...channel, online_version: '0.4.0', online_at: '2026-11-27T02:43:28Z', release_checked_at: '2026-11-27T03:00:00Z' }],
  })
})

describe('微信渠道接入', () => {
  it('回到正确的渠道并立即同步微信线上发行', async () => {
    const wrapper = mountSettings({ status: 'connected', channelID: 2 })
    await flushPromises()
    expect(mocks.sync).toHaveBeenCalledExactlyOnceWith(7, 2)
    expect(wrapper.emitted('changed')).toHaveLength(1)
    expect(wrapper.text()).toContain('0.4.0')
    expect(wrapper.text()).toContain('10:43:28')
    expect(wrapper.get('[aria-label="AppSecret"]').element.value).toBe('')
    wrapper.unmount()
  })

  it('授权缺少版本权限时给出具体错误', async () => {
    const wrapper = mountSettings({ status: 'missing-scope', channelID: 2 })
    await flushPromises()
    expect(mocks.sync).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('权限集 18')
    wrapper.unmount()
  })
})
