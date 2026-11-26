const runtimeStates = {
  running: { label: '运行中', badge: 'badge-online' },
  deploying: { label: '发布中', badge: 'badge-deploying' },
  degraded: { label: '异常', badge: 'badge-danger' },
  unavailable: { label: '不可用', badge: 'badge-danger' },
  not_released: { label: '未发布', badge: 'badge-offline' },
  unknown: { label: '未知', badge: 'badge-offline' },
}

export const runtimeBadge = status => runtimeStates[status]?.badge || 'badge-danger'
export const runtimeLabel = status => runtimeStates[status]?.label || runtimeStates.unknown.label
