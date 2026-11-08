export const stages = [
  { id: 'idea', label: '想法' }, { id: 'developing', label: '开发中' },
  { id: 'operating', label: '运营中' }, { id: 'paused', label: '暂停' }, { id: 'archived', label: '已归档' },
]
export const platforms = [
  { id: 'wechat_mini_program', label: '微信小程序' }, { id: 'douyin_mini_program', label: '抖音小程序' },
  { id: 'mini_game', label: '小游戏' }, { id: 'google_play', label: 'Google Play' },
  { id: 'app_store', label: 'App Store' }, { id: 'web', label: 'Web' }, { id: 'other', label: '其他' },
]
export const channelStatuses = [
  { id: 'draft', label: '草稿' }, { id: 'preparing', label: '准备中' }, { id: 'ready', label: '待发布' },
  { id: 'in_review', label: '审核中' }, { id: 'approved', label: '审核通过' }, { id: 'published', label: '已发布' },
  { id: 'rejected', label: '审核拒绝' }, { id: 'blocked', label: '发布受阻' }, { id: 'paused', label: '暂停发行' },
]
export const linkKinds = [
  { id: 'repository', label: '代码仓库' }, { id: 'design', label: '设计稿' },
  { id: 'requirements', label: '需求文档' }, { id: 'documentation', label: '技术文档' }, { id: 'other', label: '其他资料' },
]
export const labelFor = (options, id) => options.find(item => item.id === id)?.label || id
export const stageClass = id => ({ operating: 'badge-online', developing: 'badge-deploying', paused: 'badge-warn', archived: 'badge-offline' })[id] || 'badge-offline'
export const channelClass = id => ({ published: 'badge-online', approved: 'badge-online', in_review: 'badge-deploying', rejected: 'badge-danger', blocked: 'badge-danger', ready: 'badge-warn' })[id] || 'badge-offline'
export const dueClass = state => state === 'overdue' ? 'badge-danger' : ['today', 'soon'].includes(state) ? 'badge-warn' : 'badge-offline'
export const dateOnly = value => value ? value.slice(0, 10) : ''
export function localDate(now = new Date()) {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}
export function dueState(value, now = new Date()) {
  const due = dateOnly(value)
  if (!due) return ''
  const today = localDate(now)
  const upcoming = new Date(now)
  upcoming.setDate(upcoming.getDate() + 7)
  if (due < today) return 'overdue'
  if (due === today) return 'today'
  if (due <= localDate(upcoming)) return 'soon'
  return 'later'
}
export const dueLabel = state => ({ overdue: '已逾期', today: '今天到期', soon: '7 天内到期', later: '计划中' })[state] || '未设截止日期'
export function workbenchSummary(products, now = new Date()) {
  const active = products.filter(p => ['idea', 'developing', 'operating'].includes(p.stage))
  const actions = active.filter(p => p.next_action).sort((a, b) => (a.next_due_date || '9999').localeCompare(b.next_due_date || '9999'))
  const due = actions.filter(p => ['overdue', 'today', 'soon'].includes(dueState(p.next_due_date, now)))
  const attention = active.flatMap(product => (product.channels || [])
    .filter(channel => ['ready', 'in_review', 'approved', 'rejected', 'blocked'].includes(channel.publish_status))
    .map(channel => ({ product, channel })))
  return { active, actions, due, attention }
}
export function editableProduct(product = {}) {
  return {
    name: product.name || '', summary: product.summary || '', identifier: product.identifier || '', icon_url: product.icon_url || '',
    stage: product.stage || 'idea', links: JSON.parse(JSON.stringify(product.links || [])), next_action: product.next_action || '',
    next_due_date: dateOnly(product.next_due_date), public_bio: product.public_bio || '', public_url: product.public_url || '',
    public_showcase: Boolean(product.public_showcase), channels: JSON.parse(JSON.stringify(product.channels || [])),
    project_ids: (product.projects || []).map(item => item.id), application_ids: (product.applications || []).map(item => item.id),
  }
}
