import { api } from './index.js'

const project = productID => `/products/${encodeURIComponent(productID)}/wechat`
const channel = (productID, channelID) => `${project(productID)}/${encodeURIComponent(channelID)}`

export const getWeChatSettings = (productID, options) => api.get(project(productID), options)
export const saveWeChatConnection = (productID, channelID, data) => api.put(channel(productID, channelID), data)
export const deleteWeChatConnection = (productID, channelID) => api.delete(channel(productID, channelID))
export const authorizeWeChatRelease = (productID, channelID) => api.post(`${channel(productID, channelID)}/authorize`, {})
export const syncWeChatRelease = (productID, channelID) => api.post(`${channel(productID, channelID)}/releases/sync`, {})
export function getWeChatAnalytics(productID, channelID, days, refresh, options) {
  const path = `${channel(productID, channelID)}/analytics?days=${encodeURIComponent(days)}`
  return refresh ? api.post(path, {}, options) : api.get(path, options)
}
export const generateWeChatCode = (productID, channelID, scene) => api.post(`${channel(productID, channelID)}/code`, { scene })
export const weChatChannels = product => (product.channels || []).filter(item => ['wechat_mini_program', 'mini_game'].includes(item.platform))
