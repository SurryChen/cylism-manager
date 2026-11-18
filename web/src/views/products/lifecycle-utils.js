import { dueState, workbenchSummary } from './product-utils.js'
export const taskKinds = [{ id: 'requirement', label: '需求' }, { id: 'bug', label: '缺陷' }, { id: 'action', label: '行动' }]
export const taskStatuses = [{ id: 'todo', label: '待办' }, { id: 'in_progress', label: '进行中' }, { id: 'blocked', label: '受阻' }, { id: 'done', label: '已完成' }, { id: 'cancelled', label: '已取消' }]
export const priorities = [{ id: 'high', label: '高' }, { id: 'normal', label: '普通' }, { id: 'low', label: '低' }]
export const versionStatuses = [{ id: 'planning', label: '规划中' }, { id: 'developing', label: '开发中' }, { id: 'testing', label: '验证中' }, { id: 'releasing', label: '发行中' }, { id: 'completed', label: '已完成' }, { id: 'archived', label: '已归档' }]
export const lifecycleClass = status => ['blocked', 'rejected'].includes(status) ? 'badge-danger' : ['done', 'completed', 'published', 'approved'].includes(status) ? 'badge-online' : ['in_progress', 'developing', 'testing', 'releasing', 'in_review'].includes(status) ? 'badge-deploying' : 'badge-offline'
export const distributionStatuses = [{ id: 'draft', label: '草稿' }, { id: 'preparing', label: '准备中' }, { id: 'ready', label: '待发布' }, { id: 'in_review', label: '审核中' }, { id: 'approved', label: '审核通过' }, { id: 'published', label: '已发布' }, { id: 'rejected', label: '审核拒绝' }, { id: 'blocked', label: '发布受阻' }, { id: 'cancelled', label: '已取消' }]

export function developmentSummary(data = {}, now = new Date()) {
  const summary = workbenchSummary(data.products || [], now)
  const activeIDs = new Set(summary.active.map(product => product.id))
  const tasks = (data.tasks || []).filter(task => activeIDs.has(task.product_id) && !['done', 'cancelled'].includes(task.status))
  const legacy = summary.actions.map(product => ({ id: `legacy-${product.id}`, product_id: product.id, title: product.next_action, due_date: product.next_due_date, status: 'todo', legacy: true }))
  const rank = task => task.status === 'blocked' ? 0 : dueState(task.due_date, now) === 'overdue' ? 1 : ['today', 'soon'].includes(dueState(task.due_date, now)) ? 2 : 3
  const actions = [...tasks, ...legacy].sort((a, b) => rank(a) - rank(b) || (a.due_date || '9999').localeCompare(b.due_date || '9999'))
  const distributions = (data.distributions || []).filter(record => activeIDs.has(record.product_id))
  // Channel snapshots are the fallback for pre-existing data, never counted twice.
  const channelKeys = new Set(distributions.map(record => `${record.product_id}:${record.channel_id}`))
  const followups = [...distributions, ...summary.attention.filter(({ product, channel }) => !channelKeys.has(`${product.id}:${channel.id}`)).map(({ product, channel }) => ({ id: `legacy-${channel.id}`, product_id: product.id, channel_id: channel.id, status: channel.publish_status, platform_version: channel.pending_version, updated_at: channel.updated_at, legacy: true }))]
    .filter(record => ['ready', 'in_review', 'approved', 'rejected', 'blocked'].includes(record.status))
    .sort((a, b) => (['blocked', 'rejected'].includes(a.status) ? 0 : a.status === 'in_review' ? 2 : 1) - (['blocked', 'rejected'].includes(b.status) ? 0 : b.status === 'in_review' ? 2 : 1))
  return { ...summary, actions, due: actions.filter(task => ['overdue', 'today', 'soon'].includes(dueState(task.due_date, now))), followups, waiting: followups.filter(record => record.status === 'in_review'), needsAction: followups.filter(record => record.status !== 'in_review'), versions: (data.versions || []).filter(version => activeIDs.has(version.product_id) && !['completed', 'archived'].includes(version.status)), unplanned: summary.active.filter(product => !actions.some(task => task.product_id === product.id)) }
}
