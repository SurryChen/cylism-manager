import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WeChatChannelSettings from './WeChatChannelSettings.vue'

const mocks = vi.hoisted(() => ({ settings: vi.fn(), save: vi.fn(), code: vi.fn() }))
vi.mock('../../api/wechat.js', async importOriginal => ({
  ...await importOriginal(),
  getWeChatSettings: mocks.settings,
  saveWeChatConnection: mocks.save,
  generateWeChatCode: mocks.code,
}))

const product = { id: 7, channels: [{ id: 2, name: '小游戏', platform: 'mini_game' }] }
const channel = { channel_id: 2, app_id: 'wx1234567890abcdef' }
const mountSettings = () => mount(WeChatChannelSettings, {
  props: { product, open: true },
  global: { stubs: { Teleport: true } },
})

beforeEach(() => {
  vi.resetAllMocks()
  mocks.settings.mockResolvedValue({ connections: [channel] })
})

describe('微信运营与推广', () => {
  it('保留旧连接的 AppID，补填 AppSecret 后开放推广码入口', async () => {
    const wrapper = mountSettings()
    await flushPromises()
    expect(wrapper.get('[aria-label="AppID"]').element.value).toBe(channel.app_id)
    expect(wrapper.get('[aria-label="AppSecret"]').element.required).toBe(true)
    expect(wrapper.text()).not.toContain('扫码授权')
    expect(wrapper.text()).not.toContain('版本与审核同步')
    expect(wrapper.find('[aria-label="推广场景"]').exists()).toBe(false)
    mocks.settings.mockResolvedValue({ connections: [{ ...channel, analytics_connected: true }] })
    await wrapper.get('[aria-label="AppSecret"]').setValue('game-secret')
    await wrapper.get('#wechat-channel-settings').trigger('submit')
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledExactlyOnceWith(7, 2, { app_id: channel.app_id, app_secret: 'game-secret' })
    expect(wrapper.get('[aria-label="AppSecret"]').element.value).toBe('')
    expect(wrapper.find('[aria-label="推广场景"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('通过已配置的渠道生成并下载推广码', async () => {
    mocks.settings.mockResolvedValue({ connections: [{ ...channel, analytics_connected: true }] })
    mocks.code.mockResolvedValue({ mime: 'image/png', image: 'encoded' })
    const wrapper = mountSettings()
    await flushPromises()
    expect(wrapper.get('[aria-label="AppSecret"]').element.required).toBe(false)
    await wrapper.get('[aria-label="推广场景"]').setValue('poster-01')
    await wrapper.get('form.wechat-actions').trigger('submit')
    await flushPromises()
    expect(mocks.code).toHaveBeenCalledExactlyOnceWith(7, 2, 'poster-01')
    expect(wrapper.get('a[download]').attributes('download')).toBe('wechat-poster-01.png')
    wrapper.unmount()
  })
})
