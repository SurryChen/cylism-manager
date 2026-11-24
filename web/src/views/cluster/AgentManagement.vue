<template>
  <SurfaceCard class="agent-management-card" padding="none">
    <div class="agent-management-toolbar">
      <div>
        <strong>Cluster Agent</strong>
        <span class="cell-secondary">运行在控制平面主机上，代理本机 Kubernetes API</span>
      </div>
      <span class="badge" :class="statusClass">{{ statusLabel }}</span>
    </div>
    <EmptyState v-if="!controlNode" message="当前集群还没有控制平面，无法管理 Agent" />
    <template v-else>
      <div class="agent-management-body">
        <div class="agent-host-summary">
          <div class="agent-host-icon"><ServerCog :size="22" /></div>
          <div class="agent-host-copy">
            <span class="agent-kicker">控制平面</span>
            <strong>{{ displayName(controlNode) }}</strong>
            <span class="cell-secondary">{{ controlNode.host }}</span>
          </div>
        </div>
        <div class="agent-info-grid">
          <div class="agent-info-item"><span>监听地址</span><strong>0.0.0.0:9443</strong><small>HTTPS 双向 TLS</small></div>
          <div class="agent-info-item"><span>本机 Kubernetes API</span><strong>127.0.0.1:6443</strong><small>仅由控制平面本机代理</small></div>
          <div class="agent-info-item"><span>健康检查</span><strong>/healthz</strong><small>https://{{ controlNode.host }}:9443/healthz</small></div>
          <div class="agent-info-item"><span>系统服务</span><strong>cylism-cluster-agent</strong><small>systemd · 自动重启</small></div>
        </div>
        <div class="agent-operation-section">
          <div class="agent-section-heading"><strong>运维操作</strong><span class="cell-secondary">操作会生成集群任务并记录执行日志</span></div>
          <div class="agent-actions">
            <button class="btn btn-primary" type="button" title="上传并启动 cluster-agent" :disabled="disabled" @click="runOperation('install')">部署 Agent</button>
            <button class="btn" type="button" title="上传最新构建产物并重启 cluster-agent" :disabled="disabled" @click="runOperation('upgrade')">升级</button>
            <button class="btn" type="button" title="检查 Agent 健康接口" :disabled="disabled" @click="runOperation('test')">测试连接</button>
            <button class="btn" type="button" title="重启 cluster-agent systemd 服务" :disabled="disabled" @click="runOperation('restart')">重启</button>
            <button class="btn" type="button" title="停止 cluster-agent systemd 服务" :disabled="disabled" @click="runOperation('stop')">停止</button>
            <button class="btn btn-danger" type="button" title="停止并删除 cluster-agent 文件和服务" :disabled="disabled" @click="runOperation('uninstall')">卸载 Agent</button>
          </div>
        </div>
        <div v-if="latestTask" class="agent-latest-task"><span>最近任务：{{ taskLabel(latestTask) }}</span><span class="badge" :class="taskStatusClass(latestTask.status)">{{ taskStatusLabel(latestTask.status) }}</span></div>
      </div>
    </template>
  </SurfaceCard>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, ref } from 'vue'
import { ServerCog } from 'lucide-vue-next'
import { startClusterAgentOperation } from '../../api/clusters.js'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({
  clusterId: { type: String, required: true },
  controlNode: { type: Object, default: null },
  deployments: { type: Array, default: () => [] },
  runningDeployment: { type: Boolean, default: false },
})
const emit = defineEmits(['operation-started'])
const busy = ref(false)
const error = ref('')
const latestTask = computed(() => props.deployments.filter(task => task.operation?.startsWith('agent-') && Number(task.server_id) === Number(props.controlNode?.id)).sort((a, b) => Number(b.id) - Number(a.id))[0])
const disabled = computed(() => busy.value || props.runningDeployment)
const statusLabel = computed(() => {
  if (busy.value || latestTask.value?.status === 'running') return '执行中'
  if (latestTask.value?.status === 'success') return '已配置'
  if (latestTask.value?.status === 'failed') return '上次失败'
  return '未部署'
})
const statusClass = computed(() => ({ 'badge-deploying': statusLabel.value === '执行中', 'badge-online': statusLabel.value === '已配置', 'badge-danger': statusLabel.value === '上次失败', 'badge-offline': statusLabel.value === '未部署' }))

async function runOperation(operation) {
  if (!props.controlNode || disabled.value) return
  busy.value = true
  error.value = ''
  try {
    await startClusterAgentOperation(props.clusterId, props.controlNode.id, operation)
    emit('operation-started')
  } catch (err) {
    error.value = err.message || '启动 Agent 运维任务失败'
  } finally {
    busy.value = false
  }
}

function displayName(server) { return server.hostname || server.name || server.host || `服务器 #${server.id}` }
function taskLabel(task) { return ({ 'agent-install': '部署 Agent', 'agent-upgrade': '升级 Agent', 'agent-restart': '重启 Agent', 'agent-stop': '停止 Agent', 'agent-uninstall': '卸载 Agent', 'agent-test': '测试 Agent 连接' })[task.operation] || 'Agent 运维' }
function taskStatusLabel(status) { return ({ running: '执行中', success: '成功', failed: '失败' })[status] || status }
function taskStatusClass(status) { return ({ running: 'badge-deploying', success: 'badge-online', failed: 'badge-danger' })[status] || 'badge-offline' }
</script>

<style scoped>
.agent-management-card { margin-top: var(--tabbed-page-content-gap); }
.agent-management-toolbar { display: flex; min-height: 70px; align-items: center; justify-content: space-between; gap: 16px; padding: 14px var(--space-20); border-bottom: 1px solid var(--border-muted); }
.agent-management-toolbar > div { display: grid; gap: 4px; }
.agent-management-toolbar strong { font-size: 14px; }
.agent-management-body { display: grid; gap: 20px; padding: var(--space-20); }
.agent-host-summary { display: flex; align-items: center; gap: 12px; }
.agent-host-icon { display: grid; width: 44px; height: 44px; place-items: center; border-radius: var(--radius-control); background: var(--surface-hover); color: var(--action-primary); }
.agent-host-copy { display: grid; min-width: 0; gap: 3px; }
.agent-host-copy strong { font-size: 14px; overflow-wrap: anywhere; }
.agent-kicker { color: var(--action-primary); font-size: 11px; font-weight: 700; }
.agent-info-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.agent-info-item { display: grid; min-width: 0; gap: 5px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 12px; background: var(--surface-subtle); }
.agent-info-item span, .agent-info-item small { color: var(--text-muted); font-size: 11px; }
.agent-info-item strong { min-width: 0; color: var(--text-primary); font: 12px var(--font-mono); overflow-wrap: anywhere; }
.agent-operation-section { display: grid; gap: 10px; border-top: 1px solid var(--border-muted); padding-top: 18px; }
.agent-section-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.agent-section-heading strong { font-size: 13px; }
.agent-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.agent-latest-task { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--text-secondary); font-size: 12px; }
@media (max-width: 840px) { .agent-info-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .agent-management-toolbar, .agent-management-body { padding-right: 14px; padding-left: 14px; } .agent-info-grid { grid-template-columns: 1fr; } .agent-section-heading { align-items: flex-start; flex-direction: column; } }
</style>
