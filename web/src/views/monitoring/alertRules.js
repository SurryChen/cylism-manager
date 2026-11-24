export const alertRuleOptions = [
  { id: 'NodeDown', label: '节点不可达' }, { id: 'NodeCPUHigh', label: '节点 CPU 过高' },
  { id: 'NodeMemoryHigh', label: '节点内存过高' }, { id: 'NodeDiskHigh', label: '根磁盘空间不足' },
  { id: 'PodFrequentRestarts', label: 'Pod 频繁重启' }, { id: 'PodPending', label: 'Pod 持续 Pending' },
  { id: 'WorkloadReplicasUnavailable', label: '工作负载副本不足' }, { id: 'MonitoringTargetDown', label: '监控组件不可用' },
]
export function alertRuleLabel(name) { return alertRuleOptions.find(rule => rule.id === name)?.label || name }
